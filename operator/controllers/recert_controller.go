/*
Copyright 2021.

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

package controllers

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	recert5v1 "github.com/uberscott/recert5.git/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"
)

// RecertReconciler reconciles a Recert object
type RecertReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=recert5.uberscott.com,resources=recerts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=recert5.uberscott.com,resources=recerts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=recert5.uberscott.com,resources=recerts/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Recert object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *RecertReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx)

	reqLogger.Info("Reconciling Cert")

	instance := &recert5v1.Recert{}

	err := r.Client.Get(ctx, req.NamespacedName, instance)

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

	if instance.Status.State == "" {
		return r.reconcileNone(instance, ctx, reqLogger)
	} else if instance.Status.State == recert5v1.Pending {
		return r.reconcilePending(instance, ctx, reqLogger)
	} else if instance.Status.State == recert5v1.Creating {
		return r.reconcileCreating(instance, ctx, reqLogger)
	} else if instance.Status.State == recert5v1.FailureBackoff {
		return r.reconcileFailureBackoff(instance, ctx, reqLogger)
	} else if instance.Status.State == recert5v1.Updated {
		return r.reconcileFailureBackoff(instance, ctx, reqLogger)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RecertReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&recert5v1.Recert{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

func (r *RecertReconciler) reconcileNone(instance *recert5v1.Recert, ctx context.Context, reqLogger logr.Logger) (reconcile.Result, error) {
	if err := r.changeCertState(instance, ctx, recert5v1.Pending, reqLogger); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{Requeue: true}, nil
}

func (r *RecertReconciler) reconcilePending(instance *recert5v1.Recert, ctx context.Context, reqLogger logr.Logger) (reconcile.Result, error) {

	job := r.createRecertAgentPod(instance, reqLogger)

	if err := controllerutil.SetControllerReference(instance, job, r.Scheme); err != nil {
		return reconcile.Result{}, err
	}

	exists, err := r.existsByName(&v1.Job{}, ctx, agentName(instance), instance)

	if err != nil {
		return reconcile.Result{}, err
	} else if !exists {
		reqLogger.Info(fmt.Sprintf("Creating %v job.", job.Name))
		err = r.Client.Create(ctx, job)
		for i := 0; i < 6; i++ {
			_, err = r.findJob(instance, ctx)
			if err == nil {
				break
			}
			reqLogger.Info("waiting for job to be ready...")
			time.Sleep(5 * time.Second)
		}

		if err != nil {
			return reconcile.Result{}, err
		}

		if err := r.changeCertState(instance, ctx, recert5v1.Creating, reqLogger); err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{Requeue: true, RequeueAfter: time.Second * 30}, nil
	} else {
		reqLogger.Info("Pending job exists, waiting another 5 minutes")
		// if the job already exists then another cert is being refreshed
		// requeue and check 5 minutes later
		return reconcile.Result{Requeue: true, RequeueAfter: GetCertFailureBackoffSeconds(r.Client)}, nil
	}
}

func (r *RecertReconciler) reconcileCreating(instance *recert5v1.Recert, ctx context.Context, reqLogger logr.Logger) (reconcile.Result, error) {

	job, err := r.findJob(instance, ctx)

	// if we can't find the job, then something went really wrong FailureBackoff
	if err != nil {
		r.changeCertState(instance, ctx, recert5v1.FailureBackoff, reqLogger)
		reqLogger.Error(err, "Job could not be found.")
		return reconcile.Result{Requeue: true, RequeueAfter: time.Second * GetCertFailureBackoffSeconds(r.Client)}, err
	}

	if job.Status.Failed > 0 {
		r.changeCertState(instance, ctx, recert5v1.FailureBackoff, reqLogger)
		reqLogger.Info("Job failed.")
		r.Client.Delete(ctx, job)
		return reconcile.Result{Requeue: true, RequeueAfter: time.Second * GetCertFailureBackoffSeconds(r.Client)}, err
	}

	if job.Status.Succeeded <= 0 {
		reqLogger.Info("Job is still running.")
		return reconcile.Result{Requeue: true, RequeueAfter: 15 * time.Second}, err
	}

	// delete the old job
	r.Client.Delete(ctx, job)

	// delete leftover pods
	var pod corev1.Pod
	pod.Namespace = instance.Namespace
	err = r.Client.DeleteAllOf(ctx, &pod, &client.DeleteAllOfOptions{
		ListOptions: client.ListOptions{Namespace: instance.Namespace, LabelSelector: labels.SelectorFromSet(r.createAgentPodLabels(instance))},
	})

	if err != nil {
		reqLogger.Error(err, "could not delete job pods")
	}

	// if we have gotten to this point the job must have succeeded
	newSecret, err := r.findNewSecret(instance, ctx)
	secret, err := r.findSecret(instance, ctx)

	if err != nil {
		reqLogger.Error(err, "could not find the new newSecret")
		r.changeCertState(instance, ctx, recert5v1.FailureBackoff, reqLogger)
		return reconcile.Result{Requeue: true, RequeueAfter: time.Second * GetCertFailureBackoffSeconds(r.Client)}, err
	}

	secret.Data = newSecret.Data

	err = r.Client.Delete(ctx, newSecret)

	if err != nil {
		reqLogger.Error(err, "cannot delete new secret")
	}

	r.Client.Delete(ctx, newSecret)
	err = r.Client.Update(ctx, secret)

	if err != nil {
		reqLogger.Error(err, "cannot update secret")
		r.changeCertState(instance, ctx, recert5v1.FailureBackoff, reqLogger)
		return reconcile.Result{Requeue: true, RequeueAfter: time.Second * GetCertFailureBackoffSeconds(r.Client)}, err
	}

	err = r.changeCertState(instance, ctx, recert5v1.Updated, reqLogger)

	deployment, err := r.findSslProxyDeployment(instance, ctx)

	if err == nil {
		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = make(map[string]string)
		}
		deployment.Spec.Template.Annotations["updated"] = time.Now().String()
		err = r.Client.Update(ctx, deployment)
		if err != nil {
			reqLogger.Error(err, "could not update sslProxy deployment")
		}
	} else {
		reqLogger.Error(err, "cannot find sslProxy deployment")
	}

	// requeue once per day
	return reconcile.Result{Requeue: true, RequeueAfter: 24 * 60 * 60}, nil
}

func (r *RecertReconciler) reconcileFailureBackoff(instance *recert5v1.Recert, ctx context.Context, reqLogger logr.Logger) (reconcile.Result, error) {

	reqLogger.Info("processing FailureBackoff")
	backoffSeconds := GetCertFailureBackoffSeconds(r.Client)
	elapsed := time.Now().Unix() - instance.Status.LastStateChange

	reqLogger.Info(fmt.Sprintf("ELAPSED: %v BACKOFFSECONDS: %v", elapsed, int(backoffSeconds.Seconds())))

	if elapsed < int64(backoffSeconds.Seconds()) {
		reqLogger.Info("backoff threshold not yet reached, requeue...")
		return reconcile.Result{Requeue: true, RequeueAfter: time.Minute * 5}, nil
	}

	err := r.changeCertState(instance, ctx, recert5v1.Pending, reqLogger)
	return reconcile.Result{Requeue: true}, err
}

func (r *RecertReconciler) reconcileUpdated(instance *recert5v1.Recert, ctx context.Context, reqLogger logr.Logger) (reconcile.Result, error) {

	reqLogger.Info("processing Updated")
	renewIntervalSeconds := GetRenewInterval(r.Client)
	elapsed := time.Now().Unix() - instance.Status.LastStateChange

	reqLogger.Info(fmt.Sprintf("ELAPSED: %v BACKOFFSECONDS: %v", elapsed, int(renewIntervalSeconds.Seconds())))

	if elapsed < int64(renewIntervalSeconds.Seconds()) {
		reqLogger.Info("update threshold not yet reached, requeue...")
		return reconcile.Result{Requeue: true, RequeueAfter: renewIntervalSeconds}, nil
	}

	err := r.changeCertState(instance, ctx, recert5v1.Pending, reqLogger)
	return reconcile.Result{Requeue: true, RequeueAfter: GetUpdateRequeueDelay(r.Client)}, err
}

/////////////////////////////////////
// NAMES
/////////////////////////////////////

func agentName(instance *recert5v1.Recert) string {
	return AgentName(instance)
}

func secretName(instance *recert5v1.Recert) string {
	return SecretNameFromCert(instance)
}

func newSecretName(instance *recert5v1.Recert) string {
	return NewSecretNameFromCert(instance)
}

func sslDeploymentName(instance *recert5v1.Recert) string {
	return SslProxyDeploymentNameFromCert(instance)
}

/////////////////////////////////////
// SUB RESOURCE CREATION
/////////////////////////////////////

func (r *RecertReconciler) createAgentPodLabels(cr *recert5v1.Recert) map[string]string {
	return map[string]string{
		"certbot": cr.Spec.SslReverseProxy,
	}
}

func (r *RecertReconciler) createRecertAgentPod(cr *recert5v1.Recert, reqLogger logr.Logger) *v1.Job {

	certbotImage := "docker.io/uberscott/recert5-certbot:" + VERSION + RELEASE
	serviceAccountName, err := GetServiceAccount(r.Client, context.TODO())

	if err != nil {
		reqLogger.Error(err, "cannot find ServiceAccountName")
	}

	reqLogger.Info("Creating Recert Agent Pod with Service Account: " + serviceAccountName)

	labels := r.createAgentPodLabels(cr)

	var backoffLimit int32 = 0
	var activeDeadlineSeconds int64 = 60 * 5

	return &v1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentName(cr),
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: v1.JobSpec{
			BackoffLimit:          &backoffLimit,
			ActiveDeadlineSeconds: &activeDeadlineSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:   agentName(cr),
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccountName,
					Containers: []corev1.Container{
						{
							Name:  "certbot",
							Image: certbotImage,
							Command: []string{"/opt/uberscott/launcher.sh",
								GetCertCreateMode(r.Client),
								cr.Spec.Domain,
								cr.Spec.Email,
								cr.Name + "-nginx-sslproxy"},

							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "pvc",
									MountPath: "/etc/letsencrypt",
									ReadOnly:  false,
								},
								{
									Name:      "ssl",
									MountPath: "/ssl",
									ReadOnly:  true,
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{
						{
							Name:         "pvc",
							VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: cr.Spec.SslReverseProxy + "-nginx-ssl-reverse-proxy"}},
						},
						{
							Name:         "ssl",
							VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: cr.Spec.SslReverseProxy + "-nginx-ssl-reverse-proxy"}},
						},
					},
				},
			},
		},
	}
}

/////////////////////////////////////
//  UTILITY
/////////////////////////////////////

func (r *RecertReconciler) changeCertState(instance *recert5v1.Recert, ctx context.Context, state string, reqLogger logr.Logger) error {

	prevState := instance.Status.State
	reqLogger.Info("change cert status from: "+prevState+" to "+state)

	// first lets get the latest instance
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name}, instance)

	instance.Status.State = state

	if state == recert5v1.Updated {
		instance.Status.LastUpdated = time.Now().String()
	}

	instance.Status.LastStateChange = time.Now().Unix()

	err = r.Client.Status().Update(ctx, instance)

	if err != nil {
		reqLogger.Error(err, "error when attempting to change Cert status")
		return err
	}

	if prevState == "" {
		prevState = "None"
	}

	reqLogger.Info(fmt.Sprintf("Status going from %v to %v", prevState, state), "PrevStatus", prevState, "NewStatus", state)
	return nil
}

func (r *RecertReconciler) exists(obj client.Object, ctx context.Context, instance *recert5v1.Recert) (bool, error) {
	err := r.Client.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, obj)
	if err != nil && errors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, err
	} else {
		return true, nil
	}

}

func (r *RecertReconciler) existsByName(obj client.Object, ctx context.Context, name string, instance *recert5v1.Recert) (bool, error) {
	err := r.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: instance.Namespace}, obj)
	if err != nil && errors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, err
	} else {
		return true, nil
	}

}

func (r *RecertReconciler) findJob(instance *recert5v1.Recert, ctx context.Context) (*v1.Job, error) {
	var found v1.Job
	err := r.Client.Get(ctx, types.NamespacedName{Name: agentName(instance), Namespace: instance.Namespace}, &found)
	return &found, err
}

func (r *RecertReconciler) findNewSecret(instance *recert5v1.Recert, ctx context.Context) (*corev1.Secret, error) {
	return r.findSecretByName(instance, ctx, newSecretName(instance))
}

func (r *RecertReconciler) findSecret(instance *recert5v1.Recert, ctx context.Context) (*corev1.Secret, error) {
	return r.findSecretByName(instance, ctx, secretName(instance))
}

func (r *RecertReconciler) findSecretByName(instance *recert5v1.Recert, ctx context.Context, name string) (*corev1.Secret, error) {
	var found corev1.Secret
	err := r.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: instance.Namespace}, &found)
	return &found, err
}

func (r *RecertReconciler) findSslProxyDeployment(instance *recert5v1.Recert, ctx context.Context) (*appsv1.Deployment, error) {
	var found appsv1.Deployment
	err := r.Client.Get(ctx, types.NamespacedName{Name: sslDeploymentName(instance), Namespace: instance.Namespace}, &found)
	return &found, err
}
