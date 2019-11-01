package configmap

import (
	"context"

	"github.com/ashutoshgngwr/config-reflector/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_configmap")

// Add creates a new ConfigMap Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileConfigMap{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("configmap-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to ConfigMaps that have controller specific annotations
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return utils.HasControllerAnnotations(e.Meta)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return utils.HasControllerAnnotations(e.MetaOld)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return utils.HasControllerAnnotations(e.Meta)
		},
	})
	if err != nil {
		return err
	}

	var mapper handler.ToRequestsFunc
	mapper = func(object handler.MapObject) []reconcile.Request {
		labels := object.Meta.GetLabels()
		return []reconcile.Request{
			reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      labels[utils.LabelSourceName],
					Namespace: labels[utils.LabelSourceNamespace],
				},
			},
		}
	}

	// Watch for delete events to "reflected" ConfigMaps
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: mapper,
	}, predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool { return false },
		DeleteFunc: func(e event.DeleteEvent) bool {
			// reconcile source ConfigMap if one of its reflection is deleted
			return utils.HasSourceLabels(e.Meta) && !utils.HasControllerAnnotations(e.Meta)
		},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileConfigMap implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileConfigMap{}

// ReconcileConfigMap reconciles a ConfigMap object
type ReconcileConfigMap struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a ConfigMap object and makes changes based on the state read
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileConfigMap) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ConfigMap")

	// Fetch the ConfigMap instance
	instance := &corev1.ConfigMap{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// check if ConfigMap needs to be reflected
	namespaces := utils.GetReflectNamespaceList(instance.Annotations[utils.AnnotationReflectNamespaces])

	if namespaces == nil {
		return reconcile.Result{}, nil
	}

	configMap := newConfigMapFrom(instance)

	// Set ConfigMap instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, configMap, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// create/update ConfigMaps in reflect namespaces
	for _, namespace := range namespaces {
		if namespace == "" {
			continue
		}

		configMap.Namespace = namespace

		// Check if ConfigMap already exists
		found := &corev1.ConfigMap{}
		err = r.client.Get(context.TODO(), types.NamespacedName{
			Name:      configMap.Name,
			Namespace: configMap.Namespace,
		}, found)

		if err != nil && errors.IsNotFound(err) {
			reqLogger.Info("Creating a new ConfigMap",
				"ConfigMap.Namespace", configMap.Namespace, "ConfigMap.Name", configMap.Name)

			err = r.client.Create(context.TODO(), configMap.DeepCopy())
			if err != nil {
				return reconcile.Result{}, err
			}
		} else if err != nil {
			return reconcile.Result{}, err
		}

		// ConfigMap already exists - perform update
		reqLogger.Info("ConfigMap already exists, perform update",
			"ConfigMap.Namespace", configMap.Namespace, "ConfigMap.Name", configMap.Name)

		err = r.client.Update(context.TODO(), configMap.DeepCopy())
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	// delete any dangling ConfigMaps (in case a namespace was removed from reflectNamespaces list)
	configMapList := &corev1.ConfigMapList{}
	labelSelector := client.MatchingLabels{
		utils.LabelSourceName:      instance.Name,
		utils.LabelSourceNamespace: instance.Namespace,
	}
	err = r.client.List(context.TODO(), configMapList, labelSelector)

	if err != nil {
		if errors.IsNotFound(err) {
			// aha moment?
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err
	}

	for _, configMap := range configMapList.Items {
		if !utils.StringSliceContains(namespaces, configMap.Namespace) {
			err = r.client.Delete(context.TODO(), &configMap)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	return reconcile.Result{}, nil
}

// newConfigMapFrom returns a DeepCopy of source ConfigMap with Namespace set to nil
func newConfigMapFrom(src *corev1.ConfigMap) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        src.Name,
			Labels:      utils.CopyLabelsFromMeta(&src.ObjectMeta),
			Annotations: utils.CopyAnnotationsFromMeta(&src.ObjectMeta),
		},
		Data:       src.Data,
		BinaryData: src.BinaryData,
	}
}
