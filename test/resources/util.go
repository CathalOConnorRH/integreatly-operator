package resources

import (
	integreatlyv1alpha1 "github.com/integr8ly/integreatly-operator/pkg/apis/integreatly/v1alpha1"
)

func RunningInProw(inst *integreatlyv1alpha1.RHMI) bool {
	if v, ok := inst.Annotations["in_prow"]; !ok || v == "false" {
		return false
	}
	return true
}
