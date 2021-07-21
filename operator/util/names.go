package util

import recert "github.com/mightydevco-com/recert5.git/api/v1"

// AgentName the name of the Agent
func AgentName(instance *recert.Recert) string {
	return instance.Name + "-agent"
}

// SecretNameFromCert the name of the SLL secret
func SecretNameFromCert(instance *recert.Recert) string {
	return instance.Spec.SslReverseProxy + "-nginx-ssl-reverse-proxy"
}

// NewSecretNameFromCert this is the temporary name of the ssl secret before it gets
// copied back into Secret
func NewSecretNameFromCert(instance *recert.Recert) string {
	return instance.Spec.SslReverseProxy + "-nginx-ssl-reverse-proxy-new"
}

// SslProxyDeploymentNameFromCert is the name of the SSL Nginx Proxy Deployment
func SslProxyDeploymentNameFromCert(instance *recert.Recert) string {
	return instance.Spec.SslReverseProxy + "-nginx-ssl-reverse-proxy"
}
