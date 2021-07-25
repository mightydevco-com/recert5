package testoperator

import (
	"bytes"
	"text/template"
)

// TemplatesStruct holds all the templates
type TemplatesStruct struct {
	SslProxyTemplate   string
	CertTemplate       string
	ServiceTemplate    string
	DeploymentTemplate string
}

// RawTemplates holds raw text templates
var RawTemplates = TemplatesStruct{
	SslProxyTemplate: `apiVersion: mightydevco.com/v1alpha1
kind: SSLProxy
metadata:
  name: {{ .Name }}
spec:
  {{ if .IsFullTest }}
  loadBalancerIP: {{ .LoadBalancerIP }}
  {{ end }} 
  reverseProxy: http://{{ .Name }}:80
  replicas: 1`,

	CertTemplate: `apiVersion: mightydevco.com/v1alpha1
kind: Cert
metadata:
  name: {{ .Name }}
spec:
  domain: {{ .Domain }}
  email: scott@mightydevco.com
  sslProxy: {{ .Name }}`,

	ServiceTemplate: `apiVersion: v1
kind: Service
metadata:
  name: {{ .Name }}
  labels:
    app.kubernetes.io/name: {{ .Name }}
    app.kubernetes.io/instance: {{ .Name }}
spec:
  type: ClusterIP
  ports:
  - port: 80
    targetPort: 80
    protocol: TCP
    name: http
  selector:
    app.kubernetes.io/name: {{ .Name }}
    app.kubernetes.io/instance: {{ .Name }}`,

	DeploymentTemplate: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Name }}
  labels:
    app.kubernetes.io/name: {{ .Name }}
    app.kubernetes.io/instance: {{ .Name }}
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: {{ .Name }}
      app.kubernetes.io/instance: {{ .Name }}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: {{ .Name }}
        app.kubernetes.io/instance: {{ .Name }}
    spec:
      containers:
        - name: nginx
          image: nginxdemos/hello:latest

          imagePullPolicy: Always
          ports:
            - name: http
              containerPort: 80
              protocol: TCP`,
}

// OperatorOptionsConfigMap contains options pertaining to an individual test
// such as if we want the operator to do a dryrun or simulate a failure
var OperatorOptionsConfigMap = `apiVersion: v1
kind: ConfigMap
metadata:
  name: recert-operator
data:
  CERT_CREATE_MODE: {{ .CertRunMode }} 
  CERT_BACKOFF_SECONDS : "5" 
  CERT_RENEW_INTERVAL: "5" 
  CERT_UPDATE_REQUEUE_DELAY: "5"`

func from(raw string, data interface{}) string {
	tmpl, _ := template.New("tmp").Parse(raw)
	var w bytes.Buffer
	tmpl.Execute(&w, data)
	return string(w.Bytes())
}

func fromAll(data interface{}) *TemplatesStruct {

	rtn := TemplatesStruct{
		SslProxyTemplate:   from(RawTemplates.SslProxyTemplate, data),
		CertTemplate:       from(RawTemplates.CertTemplate, data),
		ServiceTemplate:    from(RawTemplates.ServiceTemplate, data),
		DeploymentTemplate: from(RawTemplates.DeploymentTemplate, data),
	}

	return &rtn
}
