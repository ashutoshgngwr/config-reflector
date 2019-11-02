package secret

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

var log = logf.Log.WithName("controller_secret")

// Add creates a new Secret Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileSecret{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("secret-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Secrets that have controller specific annotations
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForObject{}, predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return utils.HasControllerAnnotations(e.Meta)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return utils.HasControllerAnnotations(e.MetaOld) || utils.HasControllerAnnotations(e.MetaNew)
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

	// Watch for delete events to "reflected" Secrets
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: mapper,
	}, predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool { return false },
		UpdateFunc: func(e event.UpdateEvent) bool { return false },
		DeleteFunc: func(e event.DeleteEvent) bool {
			// reconcile source Secret if one of its reflection is deleted
			return utils.HasSourceLabels(e.Meta) && !utils.HasControllerAnnotations(e.Meta)
		},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileSecret implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileSecret{}

// ReconcileSecret reconciles a Secret object
type ReconcileSecret struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a Secret object and makes changes based on the state read
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileSecret) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling Secret")

	// Fetch the Secret instance
	instance := &corev1.Secret{}
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

	secret := newSecretFrom(instance)

	// Set Secret instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, secret, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	// create/update Secrets in reflect namespaces
	namespaces := utils.GetReflectNamespaceList(instance.Annotations[utils.AnnotationReflectNamespaces])
	for _, namespace := range namespaces {
		if namespace == "" {
			reqLogger.V(2).Info("found empty namespace in reflect-namespaces list")
			continue
		}

		if namespace == instance.Namespace {
			reqLogger.V(0).Info("reflect namespace should not be same as source namespace")
			continue
		}

		secret.Namespace = namespace

		// Check if Secret already exists
		found := &corev1.Secret{}
		err = r.client.Get(context.TODO(), types.NamespacedName{
			Name:      secret.Name,
			Namespace: secret.Namespace,
		}, found)

		if err != nil && errors.IsNotFound(err) {
			reqLogger.Info("Creating a new Secret",
				"Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)

			err = r.client.Create(context.TODO(), secret.DeepCopy())
			if err != nil {
				return reconcile.Result{}, err
			}
		} else if err != nil {
			return reconcile.Result{}, err
		} else {
			// Secret already exists - perform update
			reqLogger.Info("Secret already exists, perform update",
				"Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)

			err = r.client.Update(context.TODO(), secret.DeepCopy())
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	// delete any dangling Secrets (in case a namespace was removed from reflectNamespaces list)
	secretList := &corev1.SecretList{}
	labelSelector := client.MatchingLabels{
		utils.LabelSourceName:      instance.Name,
		utils.LabelSourceNamespace: instance.Namespace,
	}

	err = r.client.List(context.TODO(), secretList, labelSelector)
	if err != nil {
		if errors.IsNotFound(err) {
			// aha moment?
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err
	}

	for _, secret := range secretList.Items {
		if !utils.StringSliceContains(namespaces, secret.Namespace) {
			reqLogger.Info("deleting dangling Secret",
				"Secret.Name", secret.Name, "Secret.Namespace", secret.Namespace)

			// ensure that delete event doesn't reconcile the source secret
			utils.DeleteSourceLabels(&secret.ObjectMeta)
			err = r.client.Update(context.TODO(), &secret)
			if err != nil {
				return reconcile.Result{}, err
			}

			err = r.client.Delete(context.TODO(), &secret)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	return reconcile.Result{}, nil
}

// newSecretFrom returns a DeepCopy of source Secret with Namespace set to nil
func newSecretFrom(src *corev1.Secret) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        src.Name,
			Labels:      utils.CopyLabelsFromMeta(&src.ObjectMeta),
			Annotations: utils.CopyAnnotationsFromMeta(&src.ObjectMeta),
		},
		Type:       src.Type,
		Data:       src.Data,
		StringData: src.StringData,
	}
}
