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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

// +kubebuilder:rbac:groups=monitoring.example.com,resources=alertrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.example.com,resources=alertrules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=monitoring.example.com,resources=alertrules/finalizers,verbs=update
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *AlertRuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// Fetch the AlertRule instance
	alertRule := &monitoringv1.AlertRule{}
	if err := r.Get(ctx, req.NamespacedName, alertRule); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("AlertRule not found, checking for PrometheusRule to delete", "name", req.Name, "namespace", req.Namespace)
			return r.deletePrometheusRule(ctx, req.Namespace, req.Name)
		}
		logger.Error(err, "unable to fetch AlertRule")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// AlertRule이 삭제 중인 경우 스킵
	if !alertRule.DeletionTimestamp.IsZero() {
		logger.Info("AlertRule is being deleted, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// PrometheusRule 생성 또는 업데이트
	logger.Info("Reconciling PrometheusRule for AlertRule", "alertrule", alertRule.Name, "namespace", alertRule.Namespace)
	if err := r.reconcilePrometheusRule(ctx, alertRule); err != nil {
		logger.Error(err, "unable to reconcile PrometheusRule")
		return ctrl.Result{}, err
	}

	// Status 업데이트
	if err := r.updateStatus(ctx, alertRule); err != nil {
		logger.Error(err, "unable to update AlertRule status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcilePrometheusRule creates or updates a PrometheusRule based on AlertRule
func (r *AlertRuleReconciler) reconcilePrometheusRule(ctx context.Context, alertRule *monitoringv1.AlertRule) error {
	logger := logf.FromContext(ctx)

	prometheusRuleName := alertRule.Name

	// 기존 PrometheusRule 확인
	existingRule := &unstructured.Unstructured{}
	existingRule.SetGroupVersionKind(prometheusRuleGVK())
	existingRule.SetName(prometheusRuleName)
	existingRule.SetNamespace(alertRule.Namespace)

	err := r.Get(ctx, client.ObjectKey{Namespace: alertRule.Namespace, Name: prometheusRuleName}, existingRule)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("unable to fetch PrometheusRule: %w", err)
	}

	prometheusRule := r.createPrometheusRule(alertRule)

	if apierrors.IsNotFound(err) {
		logger.Info("Creating PrometheusRule", "name", prometheusRuleName, "namespace", alertRule.Namespace)
		if err := r.Create(ctx, prometheusRule); err != nil {
			return fmt.Errorf("unable to create PrometheusRule: %w", err)
		}
		logger.Info("Successfully created PrometheusRule", "name", prometheusRuleName)
	} else {
		logger.Info("Updating PrometheusRule", "name", prometheusRuleName, "namespace", alertRule.Namespace)

		prometheusRule.SetUID(existingRule.GetUID())
		prometheusRule.SetResourceVersion(existingRule.GetResourceVersion())

		if err := r.Update(ctx, prometheusRule); err != nil {
			return fmt.Errorf("unable to update PrometheusRule: %w", err)
		}
		logger.Info("Successfully updated PrometheusRule", "name", prometheusRuleName)
	}

	return nil
}

// createPrometheusRule creates a PrometheusRule unstructured object from AlertRule
func (r *AlertRuleReconciler) createPrometheusRule(alertRule *monitoringv1.AlertRule) *unstructured.Unstructured {
	prometheusRule := &unstructured.Unstructured{}
	prometheusRule.SetGroupVersionKind(prometheusRuleGVK())
	prometheusRule.SetName(alertRule.Name)
	prometheusRule.SetNamespace(alertRule.Namespace)

	// Labels 설정
	labels := map[string]string{
		"managed-by": "alert-rule-operator",
		"release":    "monitoring",
	}
	if alertRule.Labels != nil {
		for k, v := range alertRule.Labels {
			labels[k] = v
		}
	}
	prometheusRule.SetLabels(labels)

	// OwnerReference 설정
	ownerRef := metav1.OwnerReference{
		APIVersion: alertRule.APIVersion,
		Kind:       alertRule.Kind,
		Name:       alertRule.Name,
		UID:        alertRule.UID,
		Controller: func() *bool { b := true; return &b }(),
	}
	prometheusRule.SetOwnerReferences([]metav1.OwnerReference{ownerRef})

	// PrometheusRule spec 구성
	groups := []interface{}{
		map[string]interface{}{
			"name":  fmt.Sprintf("%s-group", alertRule.Name),
			"rules": []interface{}{r.buildPrometheusRule(alertRule)},
		},
	}

	spec := map[string]interface{}{
		"groups": groups,
	}

	if err := unstructured.SetNestedMap(prometheusRule.Object, spec, "spec"); err != nil {
		logf.Log.Error(err, "unable to set PrometheusRule spec")
	}

	return prometheusRule
}

// buildPrometheusRule builds a single Prometheus rule from AlertRule
func (r *AlertRuleReconciler) buildPrometheusRule(alertRule *monitoringv1.AlertRule) map[string]interface{} {
	rule := map[string]interface{}{
		"alert": alertRule.Spec.Alert,
		"expr":  alertRule.Spec.Expr,
	}

	if alertRule.Spec.For != "" {
		rule["for"] = alertRule.Spec.For
	}

	labels := map[string]interface{}{
		"severity": alertRule.Spec.Severity,
	}
	if alertRule.Spec.Labels != nil {
		for k, v := range alertRule.Spec.Labels {
			labels[k] = v
		}
	}
	rule["labels"] = labels

	if alertRule.Spec.Annotations != nil {
		annotations := make(map[string]interface{})
		for k, v := range alertRule.Spec.Annotations {
			annotations[k] = v
		}
		rule["annotations"] = annotations
	}

	return rule
}

// deletePrometheusRule deletes the PrometheusRule associated with an AlertRule
func (r *AlertRuleReconciler) deletePrometheusRule(ctx context.Context, namespace, alertRuleName string) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	prometheusRuleName := alertRuleName

	prometheusRule := &unstructured.Unstructured{}
	prometheusRule.SetGroupVersionKind(prometheusRuleGVK())
	prometheusRule.SetName(prometheusRuleName)
	prometheusRule.SetNamespace(namespace)

	err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: prometheusRuleName}, prometheusRule)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// PrometheusRule이 이미 없으면 스킵
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch PrometheusRule for deletion")
		return ctrl.Result{}, err
	}

	logger.Info("Deleting PrometheusRule for deleted AlertRule", "prometheusrule", prometheusRuleName)
	if err := r.Delete(ctx, prometheusRule); err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Error(err, "unable to delete PrometheusRule")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// updateStatus updates the AlertRule status
func (r *AlertRuleReconciler) updateStatus(ctx context.Context, alertRule *monitoringv1.AlertRule) error {
	// PrometheusRule 존재 여부 확인
	prometheusRule := &unstructured.Unstructured{}
	prometheusRule.SetGroupVersionKind(prometheusRuleGVK())
	prometheusRule.SetName(alertRule.Name)
	prometheusRule.SetNamespace(alertRule.Namespace)

	err := r.Get(ctx, client.ObjectKey{Namespace: alertRule.Namespace, Name: alertRule.Name}, prometheusRule)

	// Status 업데이트
	condition := metav1.Condition{
		Type:               "PrometheusRuleReady",
		Status:             metav1.ConditionTrue,
		Reason:             "PrometheusRuleCreated",
		Message:            "PrometheusRule has been successfully created",
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: alertRule.Generation,
	}

	if err != nil {
		if apierrors.IsNotFound(err) {
			condition.Status = metav1.ConditionFalse
			condition.Reason = "PrometheusRuleNotFound"
			condition.Message = "PrometheusRule not found"
		} else {
			condition.Status = metav1.ConditionUnknown
			condition.Reason = "Error"
			condition.Message = fmt.Sprintf("Error checking PrometheusRule: %v", err)
		}
	}

	// 기존 조건 업데이트 또는 추가
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

// prometheusRuleGVK returns the GroupVersionKind for PrometheusRule
func prometheusRuleGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "monitoring.coreos.com",
		Version: "v1",
		Kind:    "PrometheusRule",
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AlertRuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1.AlertRule{}).
		Named("alertrule").
		Complete(r)
}
