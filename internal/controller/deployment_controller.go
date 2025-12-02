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

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	monitoringv1 "github.com/Kim-Yukyung/k8s-alert-rule-operator/api/v1"
)

// DeploymentReconciler reconciles a Deployment object
type DeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=monitoring.example.com,resources=alertrules,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, req.NamespacedName, deployment); err != nil {
		if apierrors.IsNotFound(err) {
			// Deployment가 삭제된 경우, 관련 AlertRule도 삭제
			logger.Info("Deployment not found, checking for AlertRule to delete", "name", req.Name, "namespace", req.Namespace)
			return r.deleteAlertRuleForDeployment(ctx, req.Namespace, req.Name)
		}
		logger.Error(err, "unable to fetch Deployment")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Deployment가 삭제 중인 경우 스킵
	if !deployment.DeletionTimestamp.IsZero() {
		logger.Info("Deployment is being deleted, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	alertRuleName := fmt.Sprintf("%s-alert", deployment.Name)

	// 기존 AlertRule 확인
	alertRule := &monitoringv1.AlertRule{}
	err := r.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: alertRuleName}, alertRule)
	if err != nil && !apierrors.IsNotFound(err) {
		logger.Error(err, "unable to fetch AlertRule")
		return ctrl.Result{}, err
	}

	if apierrors.IsNotFound(err) {
		logger.Info("Creating AlertRule for Deployment", "deployment", deployment.Name, "namespace", req.Namespace)
		newAlertRule := r.createDefaultAlertRule(deployment, alertRuleName)
		if err := r.Create(ctx, newAlertRule); err != nil {
			logger.Error(err, "unable to create AlertRule")
			return ctrl.Result{}, err
		}
		logger.Info("Successfully created AlertRule", "alertrule", alertRuleName)
		return ctrl.Result{}, nil
	}

	// AlertRule이 이미 존재하는 경우, Deployment 참조 업데이트
	if alertRule.Spec.DeploymentRef == nil ||
		alertRule.Spec.DeploymentRef.Namespace != deployment.Namespace ||
		alertRule.Spec.DeploymentRef.Name != deployment.Name {
		logger.Info("Updating AlertRule deployment reference", "alertrule", alertRuleName)
		alertRule.Spec.DeploymentRef = &monitoringv1.DeploymentReference{
			Namespace: deployment.Namespace,
			Name:      deployment.Name,
		}
		if err := r.Update(ctx, alertRule); err != nil {
			logger.Error(err, "unable to update AlertRule")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// createDefaultAlertRule creates a default AlertRule for a Deployment
func (r *DeploymentReconciler) createDefaultAlertRule(deployment *appsv1.Deployment, name string) *monitoringv1.AlertRule {
	// 기본 알림 규칙 생성
	alertRule := &monitoringv1.AlertRule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: deployment.Namespace,
			Labels: map[string]string{
				"app":                           deployment.Name,
				"managed-by":                    "alert-rule-operator",
				"deployment.kubernetes.io/name": deployment.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: deployment.APIVersion,
					Kind:       deployment.Kind,
					Name:       deployment.Name,
					UID:        deployment.UID,
					Controller: func() *bool { b := true; return &b }(),
				},
			},
		},
		Spec: monitoringv1.AlertRuleSpec{
			Alert:    fmt.Sprintf("%sPodDown", deployment.Name),
			Expr:     fmt.Sprintf("up{job=\"%s\"} == 0", deployment.Name),
			For:      "1m",
			Severity: "critical",
			Labels: map[string]string{
				"deployment": deployment.Name,
				"namespace":  deployment.Namespace,
			},
			Annotations: map[string]string{
				"summary":     fmt.Sprintf("Pod %s is down", deployment.Name),
				"description": fmt.Sprintf("Pod %s in namespace %s has been down for more than 1 minutes", deployment.Name, deployment.Namespace),
			},
			DeploymentRef: &monitoringv1.DeploymentReference{
				Namespace: deployment.Namespace,
				Name:      deployment.Name,
			},
		},
	}

	// Controller reference 설정
	if err := ctrl.SetControllerReference(deployment, alertRule, r.Scheme); err != nil {
		log.Log.Error(err, "unable to set controller reference")
	}

	return alertRule
}

// deleteAlertRuleForDeployment deletes the AlertRule associated with a Deployment
func (r *DeploymentReconciler) deleteAlertRuleForDeployment(ctx context.Context, namespace, deploymentName string) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	alertRuleName := fmt.Sprintf("%s-alert", deploymentName)

	alertRule := &monitoringv1.AlertRule{}
	err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: alertRuleName}, alertRule)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// AlertRule이 이미 없으면 스킵
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch AlertRule for deletion")
		return ctrl.Result{}, err
	}
	
	logger.Info("Deleting AlertRule for deleted Deployment", "alertrule", alertRuleName)
	if err := r.Delete(ctx, alertRule); err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Error(err, "unable to delete AlertRule")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.Deployment{}).
		Named("deployment").
		Complete(r)
}
