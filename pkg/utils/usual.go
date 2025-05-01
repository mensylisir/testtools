package utils

import corev1 "k8s.io/api/core/v1"

type JobParams struct {
	Namespace      string
	Name           string
	Command        string
	Args           []string
	Labels         map[string]string
	Image          string
	NodeName       string
	NodeSelector   map[string]string
	Affinity       *corev1.Affinity
	Port           *corev1.ContainerPort
	ReadinessProbe *corev1.Probe
}
