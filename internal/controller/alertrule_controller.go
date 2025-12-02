/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1 "github.com/Kim-Yukyung/k8s-alert-rule-operator/api/v1"
)

// AlertRuleReconciler reconciles a AlertRule object
type AlertRuleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=monitoring.my.domain,resources=alertrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.my.domain,resources=alertrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=monitoring.my.domain,resources=alertrules/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/reconcile
func (r *AlertRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the AlertRule instance
	alertRule := &monitoringv1.AlertRule{}
	if err := r.Get(ctx, req.NamespacedName, alertRule); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return. Created objects are automatically garbage collected.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch AlertRule")
		return ctrl.Result{}, err
	}

	// Update status to Progressing
	if err := r.updateStatus(ctx, alertRule, metav1.Condition{
		Type:    "Progressing",
		Status:  metav1.ConditionTrue,
		Reason:  "Reconciling",
		Message: "Reconciling AlertRule",
	}); err != nil {
		log.Error(err, "unable to update status")
		return ctrl.Result{}, err
	}

	// Generate Prometheus alert rules YAML
	alertRulesYAML, err := r.generateAlertRulesYAML(alertRule)
	if err != nil {
		log.Error(err, "unable to generate alert rules YAML")
		if updateErr := r.updateStatus(ctx, alertRule, metav1.Condition{
			Type:    "Degraded",
			Status:  metav1.ConditionTrue,
			Reason:  "GenerationFailed",
			Message: fmt.Sprintf("Failed to generate alert rules: %v", err),
		}); updateErr != nil {
			log.Error(updateErr, "unable to update status")
		}
		return ctrl.Result{}, err
	}

	// Create or update ConfigMap with alert rules
	configMapName := fmt.Sprintf("alertrule-%s", alertRule.Name)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: alertRule.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "k8s-alert-rule-operator",
				"app.kubernetes.io/managed-by":  "alertrule-controller",
				"alertrule.monitoring.my.domain": alertRule.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: alertRule.APIVersion,
					Kind:       alertRule.Kind,
					Name:       alertRule.Name,
					UID:        alertRule.UID,
					Controller: func() *bool { b := true; return &b }(),
				},
			},
		},
		Data: map[string]string{
			"alertrules.yaml": alertRulesYAML,
		},
	}

	// Check if ConfigMap already exists
	existingConfigMap := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: alertRule.Namespace}, existingConfigMap)
	if err != nil && apierrors.IsNotFound(err) {
		// Create new ConfigMap
		if err := r.Create(ctx, configMap); err != nil {
			log.Error(err, "unable to create ConfigMap")
			if updateErr := r.updateStatus(ctx, alertRule, metav1.Condition{
				Type:    "Degraded",
				Status:  metav1.ConditionTrue,
				Reason:  "ConfigMapCreationFailed",
				Message: fmt.Sprintf("Failed to create ConfigMap: %v", err),
			}); updateErr != nil {
				log.Error(updateErr, "unable to update status")
			}
			return ctrl.Result{}, err
		}
		log.Info("created ConfigMap", "configmap", configMapName)
	} else if err != nil {
		log.Error(err, "unable to fetch ConfigMap")
		return ctrl.Result{}, err
	} else {
		// Update existing ConfigMap
		existingConfigMap.Data = configMap.Data
		existingConfigMap.Labels = configMap.Labels
		if err := r.Update(ctx, existingConfigMap); err != nil {
			log.Error(err, "unable to update ConfigMap")
			if updateErr := r.updateStatus(ctx, alertRule, metav1.Condition{
				Type:    "Degraded",
				Status:  metav1.ConditionTrue,
				Reason:  "ConfigMapUpdateFailed",
				Message: fmt.Sprintf("Failed to update ConfigMap: %v", err),
			}); updateErr != nil {
				log.Error(updateErr, "unable to update status")
			}
			return ctrl.Result{}, err
		}
		log.Info("updated ConfigMap", "configmap", configMapName)
	}

	// Update status to Available
	if err := r.updateStatus(ctx, alertRule, metav1.Condition{
		Type:    "Available",
		Status:  metav1.ConditionTrue,
		Reason:  "Reconciled",
		Message: fmt.Sprintf("AlertRule reconciled successfully. ConfigMap: %s", configMapName),
	}); err != nil {
		log.Error(err, "unable to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// generateAlertRulesYAML converts AlertRule to Prometheus alert rules YAML format
func (r *AlertRuleReconciler) generateAlertRulesYAML(alertRule *monitoringv1.AlertRule) (string, error) {
	var rules []string

	for _, rule := range alertRule.Spec.Rules {
		// Build annotations
		annotations := []string{
			fmt.Sprintf("    summary: \"%s\"", rule.Name),
			fmt.Sprintf("    severity: \"%s\"", rule.Severity),
		}

		// Add notification channels to annotations
		for _, notif := range rule.Notifications {
			if notif.Discord != "" {
				annotations = append(annotations, fmt.Sprintf("    discord: \"%s\"", notif.Discord))
			}
			if notif.Email != "" {
				annotations = append(annotations, fmt.Sprintf("    email: \"%s\"", notif.Email))
			}
		}

		// Build labels
		labels := []string{
			fmt.Sprintf("    alertname: \"%s\"", rule.Name),
			fmt.Sprintf("    severity: \"%s\"", rule.Severity),
			fmt.Sprintf("    deployment: \"%s\"", alertRule.Spec.TargetDeployment),
		}

		// Build alert rule YAML
		ruleYAML := fmt.Sprintf(`- alert: %s
  expr: %s
  for: %s
  labels:
%s
  annotations:
%s
`, rule.Name, rule.Condition, rule.Duration, strings.Join(labels, "\n"), strings.Join(annotations, "\n"))

		rules = append(rules, ruleYAML)
	}

	return "groups:\n- name: " + alertRule.Name + "\n  rules:\n" + strings.Join(rules, ""), nil
}

// updateStatus updates the status conditions of AlertRule
func (r *AlertRuleReconciler) updateStatus(ctx context.Context, alertRule *monitoringv1.AlertRule, condition metav1.Condition) error {
	condition.LastTransitionTime = metav1.Now()
	condition.ObservedGeneration = alertRule.Generation

	// Find existing condition
	found := false
	for i, c := range alertRule.Status.Conditions {
		if c.Type == condition.Type {
			alertRule.Status.Conditions[i] = condition
			found = true
			break
		}
	}

	if !found {
		alertRule.Status.Conditions = append(alertRule.Status.Conditions, condition)
	}

	return r.Status().Update(ctx, alertRule)
}

// SetupWithManager sets up the controller with the Manager.
func (r *AlertRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1.AlertRule{}).
		Named("alertrule").
		Complete(r)
}
