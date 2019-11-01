package utils

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CopyAnnotationsFromMeta will copy all relavant annotations from source ObjectMeta
func CopyAnnotationsFromMeta(src *metav1.ObjectMeta) map[string]string {
	annotations := make(map[string]string)

	// copy all annotations except those with "configreflector" prefix
	if src.Annotations[AnnotationReflectAnnotations] != "" {
		for key, value := range src.Annotations {
			if strings.HasPrefix(key, Prefix) {
				continue
			}

			annotations[key] = value
		}
	}

	return annotations
}

// CopyLabelsFromMeta will copy all labels from source ObjectMeta
func CopyLabelsFromMeta(src *metav1.ObjectMeta) map[string]string {
	labels := make(map[string]string)

	// copy all labels from source ConfigMap
	if src.Annotations[AnnotationReflectLabels] != "" {
		for key, value := range src.Labels {
			labels[key] = value
		}
	}

	labels[LabelSourceName] = src.Name
	labels[LabelSourceNamespace] = src.Namespace
	return labels
}

// GetReflectNamespaceList splits annotation value into a namespace list
func GetReflectNamespaceList(reflectNamespaces string) []string {
	if reflectNamespaces == "" {
		return nil
	}

	namespaces := strings.Split(reflectNamespaces, ",")
	for i := range namespaces {
		namespaces[i] = strings.TrimSpace(namespaces[i])
	}

	return namespaces
}

// StringSliceContains returns if a string slice contains a particular string
func StringSliceContains(haystack []string, needle string) bool {
	for _, str := range haystack {
		if str == needle {
			return true
		}
	}

	return false
}

// HasControllerAnnotations checks if ObjectMeta has reflect-namespaces annotation
func HasControllerAnnotations(meta metav1.Object) bool {
	return meta.GetAnnotations()[AnnotationReflectNamespaces] != ""
}

// HasSourceLabels checks if ObjectMeta has source-namespace and source-name labels
func HasSourceLabels(meta metav1.Object) bool {
	labels := meta.GetLabels()
	return labels[LabelSourceName] != "" && labels[LabelSourceNamespace] != ""
}
