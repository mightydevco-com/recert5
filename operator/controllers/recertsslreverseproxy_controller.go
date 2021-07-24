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
	"bytes"
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strings"
	"text/template"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	recert5v1 "github.com/uberscott/recert5.git/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

// RecertSSLReverseProxyReconciler reconciles a RecertSSLReverseProxy object
type RecertSSLReverseProxyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=recert5.uberscott.com,resources=recertsslreverseproxies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=recert5.uberscott.com,resources=recertsslreverseproxies/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=recert5.uberscott.com,resources=recertsslreverseproxies/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RecertSSLReverseProxy object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *RecertSSLReverseProxyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx)

	reqLogger = log.Log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling SSLProxy")

	// Fetch the SSLProxy instance
	instance := &recert5v1.RecertSSLReverseProxy{}
	err := r.Get(ctx, req.NamespacedName, instance)
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

	result, err := r.reconcilePVC(instance, ctx, reqLogger)
	if result.Requeue || err != nil {
		return result, err
	}

	result, err = r.reconcileNginxConfigMap(instance, ctx, reqLogger)
	if result.Requeue || err != nil {
		return result, err
	}

	result, err = r.reconcileSSLSecret(instance, ctx, reqLogger)
	if result.Requeue || err != nil {
		return result, err
	}

	result, err = r.reconcileService(instance, ctx, reqLogger)
	if result.Requeue || err != nil {
		return result, err
	}

	result, err = r.reconcileCertbotService(instance, ctx, reqLogger)
	if result.Requeue || err != nil {
		return result, err
	}

	result, err = r.reconcileNginxDeployment(instance, ctx, reqLogger)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RecertSSLReverseProxyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&recert5v1.RecertSSLReverseProxy{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Complete(r)
}

func (r *RecertSSLReverseProxyReconciler) reconcilePVC(instance *recert5v1.RecertSSLReverseProxy, ctx context.Context, logger logr.Logger) (reconcile.Result, error) {

	pvc := r.newSecretPvc(instance)

	// Set SSLProxy instance as the owner and controller
	if err := ctrl.SetControllerReference(instance, pvc, r.Scheme); err != nil {
		return reconcile.Result{}, err
	}

	found := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating a new PVC", "PVC.Namespace", pvc.Namespace, "PVC.Name", pvc.Name)
		err = r.Create(ctx, pvc)
		if err != nil {
			logger.Error(err, "Error when attempting to create a new PVC")
			return reconcile.Result{}, err
		}
	} else if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *RecertSSLReverseProxyReconciler) reconcileService(instance *recert5v1.RecertSSLReverseProxy, ctx context.Context, logger logr.Logger) (reconcile.Result, error) {

	service := r.newService(instance, logger)

	// Set SSLProxy instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, service, r.Scheme); err != nil {
		logger.Error(err, "error when setting the controller reference")
		return reconcile.Result{}, err
	}

	found := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating a new Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		err = r.Create(ctx, service)
		if err != nil {
			logger.Error(err, "Error when attempting to create a new Service")
			return reconcile.Result{}, err
		}
	} else if err != nil {
		logger.Error(err, "Error when creating service")
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *RecertSSLReverseProxyReconciler) reconcileCertbotService(instance *recert5v1.RecertSSLReverseProxy, ctx context.Context, logger logr.Logger) (reconcile.Result, error) {

	service := r.newCertbotService(instance)

	// Set SSLProxy instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, service, r.Scheme); err != nil {
		logger.Error(err, "error when setting the controller reference")
		return reconcile.Result{}, err
	}

	found := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating Certbot Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		err = r.Create(ctx, service)
		if err != nil {
			logger.Error(err, "Error when attempting to create a new Service")
			return reconcile.Result{}, err
		}
	} else if err != nil {
		logger.Error(err, "Error when creating service")
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *RecertSSLReverseProxyReconciler) getSSLSecret(instance *recert5v1.RecertSSLReverseProxy, ctx context.Context) (*corev1.Secret, error) {
	found := &corev1.Secret{}
	secret := r.newSSLSecret(instance)
	err := r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, found)

	if err != nil && errors.IsNotFound(err) {
		return secret, nil
	}

	return found, err
}

func (r *RecertSSLReverseProxyReconciler) reconcileSSLSecret(instance *recert5v1.RecertSSLReverseProxy, ctx context.Context, logger logr.Logger) (reconcile.Result, error) {

	secret := r.newSSLSecret(instance)

	// Set SSLProxy instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, secret, r.Scheme); err != nil {
		return reconcile.Result{}, err
	}

	found := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, found)

	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating a new NGINX Secret for SSL", "Secret.Namespace", secret.Namespace, "ConfigMap.Name", secret.Name)
		err = r.Create(ctx, secret)

		if err != nil {
			logger.Error(err, "Error when attempting to create a new NGINX configMap")
			return reconcile.Result{}, err
		}
	} else if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *RecertSSLReverseProxyReconciler) reconcileNginxConfigMap(instance *recert5v1.RecertSSLReverseProxy, ctx context.Context, logger logr.Logger) (reconcile.Result, error) {

	tmpl, err := template.New("default.conf").Parse(defaultNginxConf)

	if err != nil {
		logger.Error(err, "could not parse default.conf template")
		return reconcile.Result{}, err
	}

	var content bytes.Buffer
	data := nginxConf{ReverseProxy: instance.Spec.ReverseProxy,
		CertbotService: instance.Name + "-certbot-service"}
	err = tmpl.Execute(&content, data)

	if err != nil {
		logger.Error(err, "could not execute nginx conf template")
		return reconcile.Result{}, err
	}

	configMap := r.newNginxConfigMap(instance, string(content.Bytes()))

	// Set SSLProxy instance as the owner and controller
	if err = controllerutil.SetControllerReference(instance, configMap, r.Scheme); err != nil {
		return reconcile.Result{}, err
	}

	found := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating a new NGINX ConfigMap ", "ConfigMap.Namespace", configMap.Namespace, "ConfigMap.Name", configMap.Name)
		err = r.Create(ctx, configMap)
		if err != nil {
			logger.Error(err, "Error when attempting to create a new NGINX configMap")
			return reconcile.Result{}, err
		}
	} else if err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *RecertSSLReverseProxyReconciler) reconcileNginxDeployment(instance *recert5v1.RecertSSLReverseProxy, ctx context.Context, logger logr.Logger) (reconcile.Result, error) {

	secret, err := r.getSSLSecret(instance, ctx)

	if err != nil {
		return reconcile.Result{}, err
	}

	imagesMap, err := GetImagesConfigMap(r.Client)

	if err != nil {
		return reconcile.Result{}, err
	}

	labels := map[string]string{
		"sslproxy": instance.Name,
	}

	annotations := map[string]string{
		"sslSecretVersion": secret.ResourceVersion,
	}

	deployment := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        instance.Name + "-nginx-sslproxy",
			Namespace:   instance.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: v1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels:      labels,
				MatchExpressions: []metav1.LabelSelectorRequirement{}},
			Replicas: instance.Spec.Replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},

				Spec: corev1.PodSpec{

					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: imagesMap.Data["recertNginx"],
							VolumeMounts: []corev1.VolumeMount{
								{Name: "conf",
									ReadOnly:  true,
									MountPath: "/etc/recert/conf",
								},
								{Name: "ssl",
									ReadOnly:  true,
									MountPath: "/etc/recert/ssl",
								},
							},
						},
					},
					Volumes: []corev1.Volume{{
						Name:         "conf",
						VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: instance.Name + "-nginx-sslproxy"}}},
					},
						{
							Name:         "ssl",
							VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: instance.Name + "-nginx-sslproxy"}},
						},
					},
				}},
		},
	}

	// Set SSLProxy instance as the owner and controller
	if err := controllerutil.SetControllerReference(instance, deployment, r.Scheme); err != nil {
		return reconcile.Result{}, err
	}

	found := &v1.Deployment{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating a new NGINX Deployment", "Deployment.Namespace", deployment.Namespace, "Deployment.Name", deployment.Name)
		err = r.Client.Create(ctx, deployment)
		if err != nil {
			logger.Error(err, "Error when attempting to create a new NGINX deployment")
			return reconcile.Result{}, err
		}
		// Deployment created successfully - don't requeue
	} else if err != nil {
		return reconcile.Result{}, err
	} else if found.Annotations["sslSecretVersion"] != secret.ResourceVersion {
		logger.Info("sslSecretVersion has been updated, must update deployment")
		r.Client.Update(ctx, deployment)
	}
	return reconcile.Result{}, nil
}

func (r *RecertSSLReverseProxyReconciler) newNginxConfigMap(cr *recert5v1.RecertSSLReverseProxy, defaultConf string) *corev1.ConfigMap {
	labels := map[string]string{
		"sslproxy": cr.Name,
	}
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-nginx-sslproxy",
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Data: map[string]string{"default.conf": defaultConf},
	}
}

func (r *RecertSSLReverseProxyReconciler) newSSLSecret(cr *recert5v1.RecertSSLReverseProxy) *corev1.Secret {
	labels := map[string]string{
		"sslproxy": cr.Name,
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-nginx-sslproxy",
			Namespace: cr.Namespace,
			Labels:    labels,
		},
	}
}

func (r *RecertSSLReverseProxyReconciler) newCertbotService(cr *recert5v1.RecertSSLReverseProxy) *corev1.Service {
	labels := map[string]string{
		"certbot": cr.Name,
	}
	rtn := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-certbot-service",
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{Name: "http",
					Port:       80,
					TargetPort: intstr.FromInt(80),
					Protocol:   corev1.ProtocolTCP},
			},
			Selector: map[string]string{
				"certbot": cr.Name,
			},
		},
	}

	return rtn
}

func (r *RecertSSLReverseProxyReconciler) newService(cr *recert5v1.RecertSSLReverseProxy, log logr.Logger) *corev1.Service {
	labels := map[string]string{
		"sslproxy": cr.Name,
	}
	rtn := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-nginx-sslproxy",
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{
				{Name: "http",
					Port:       80,
					TargetPort: intstr.FromInt(80),
					Protocol:   corev1.ProtocolTCP},
				{Name: "https",
					Port:       443,
					TargetPort: intstr.FromInt(443),
					Protocol:   corev1.ProtocolTCP},
			},
			Selector: map[string]string{
				"sslproxy": cr.Name,
			},
		},
	}

	if strings.TrimSpace(cr.Spec.LoadBalancerIP) != "" {
		log.Info(fmt.Sprintf("LoadBalancerIp set to %v", cr.Spec.LoadBalancerIP))
		rtn.Spec.LoadBalancerIP = cr.Spec.LoadBalancerIP
	} else {
		log.Info("skipping Load Balancer which is not specified")
	}

	return rtn
}

func (r *RecertSSLReverseProxyReconciler) newSecretPvc(cr *recert5v1.RecertSSLReverseProxy) *corev1.PersistentVolumeClaim {
	labels := map[string]string{
		"sslproxy": cr.Name,
	}
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-nginx-sslproxy",
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{StorageClassName: getStorageClassDefault(),
			AccessModes: []corev1.PersistentVolumeAccessMode{"ReadWriteOnce"},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			}},
	}
}

type nginxConf struct {
	CertbotService string
	ReverseProxy   string
}

var defaultNginxConf = `##############################################
# 
# NGINX TEMPLATE FROM RECERT OPERATOR
#
##############################################

server 
{
   listen 80;
   listen [::]:80;

   location /.well-known/
   {
      proxy_pass http://{{ .CertbotService }}:80;
      proxy_bind $server_addr;
      proxy_set_header X-Forwarded-Host $http_host;
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_set_header X-Real-IP $remote_addr;
      proxy_set_header X-Forwarded-Port 80;
      proxy_set_header X-Forwarded-Proto $scheme;
      proxy_redirect off;
      proxy_intercept_errors on;
      add_header Access-Control-Allow-Origin *;
      expires -1;
   }
   
   location /
   {
      proxy_pass {{ .ReverseProxy }}/;
      proxy_set_header X-Forwarded-Host $http_host;
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_set_header X-Real-IP $remote_addr;
      proxy_set_header X-Forwarded-Port 443;
      proxy_set_header X-Forwarded-Proto $scheme;
      proxy_redirect off;
      proxy_intercept_errors on;
      expires -1;
   }
}

server 
{
   listen 443 ssl http2;
   listen [::]:443 ssl http2;

   ssl_certificate     /ssl/$ssl_server_name.crt;
   ssl_certificate_key /ssl/$ssl_server_name.key;

   location /
   {
      proxy_pass {{ .ReverseProxy }}/;
      proxy_set_header X-Forwarded-Host $http_host;
      proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
      proxy_set_header X-Real-IP $remote_addr;
      proxy_set_header X-Forwarded-Port 443;
      proxy_set_header X-Forwarded-Proto $scheme;
      proxy_redirect off;
      proxy_intercept_errors on;
      expires -1;
   }
}`

func getStorageClassDefault() *string {
	rtn, found := os.LookupEnv("STORAGE_CLASS")
	if !found {
		rtn = "standard"
	}
	if len(rtn) == 0 {
		rtn = "standard"
	}
	return &rtn
}
