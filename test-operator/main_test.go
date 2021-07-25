package testoperator

import (
	"fmt"
	http_helper "github.com/gruntwork-io/terratest/modules/http-helper"
	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSslProxy(t *testing.T) {
	stage := createStage()
	stage.logConf()
	stage.stage(t)

	stage.testExpectedSslProxyDefaultConf(t)
	stage.testProxyWorking(t)
	if stage.Conf.IsFullTest {

		stage.testExpectedIP(t)
	}
}

func TestCertFail(t *testing.T) {

	stage := createStage()
	stage.stage(t)
	stage.setRunOptions(t, &RunOptions{CertRunMode: "fail"})

	defer k8s.KubectlDeleteFromString(t, stage.Options, stage.Templates.CertTemplate)
	k8s.KubectlApplyFromString(t, stage.Options, stage.Templates.CertTemplate)

	stage.verifyState(t, "FailureBackoff", "FailureBackoff", 120, 1*time.Second)
	stage.verifyState(t, "Pending", "Pending", 30, 2*time.Second)
	stage.verifyState(t, "Creating", "Creating", 30, 2*time.Second)
}

func TestCert(t *testing.T) {

	stage := createStage()

	if !stage.Conf.IsFullTest {
		t.SkipNow()
		return
	}

	stage.stage(t)
	stage.setRunOptions(t, &RunOptions{CertRunMode: "dryrun"})

	stage.testExpectedIP(t)

	defer k8s.KubectlDeleteFromString(t, stage.Options, stage.Templates.CertTemplate)
	k8s.KubectlApplyFromString(t, stage.Options, stage.Templates.CertTemplate)

	// wait for Cert to go into Updated mode
	retry.DoWithRetry(t, "verify state updated", 30, 5*time.Second, func() (string, error) {
		output, err := k8s.RunKubectlAndGetOutputE(t, stage.Options, "get", "cert", stage.Conf.Name, "-o", "yaml")
		if err != nil {
			return "fail", err
		}

		var stub CertStub
		yaml.Unmarshal([]byte(output), &stub)

		if stub.Status.State != "Updated" {
			return "fail", fmt.Errorf("state is : %v", stub.Status.State)
		}

		if stub.Status.LastUpdated == "" {
			return "fail", fmt.Errorf("cert.Status.LastUpdated is not set")
		}

		return "pass", nil
	})

	// grab the latest secret
	secret := k8s.GetSecret(t, stage.Options, stage.Conf.Name+"-nginx-sslproxy")

	if secret.Data[stage.Conf.Domain+".crt"] == nil {
		t.Errorf("expected entry for %v.crt", stage.Conf.Domain)
	}

	if secret.Data[stage.Conf.Domain+".key"] == nil {
		t.Errorf("expected entry for %v.key", stage.Conf.Domain)
	}

	// wait for pod to have updated annotation which causes a restart
	retry.DoWithRetry(t, "verify sllproxy updated", 30, 1*time.Second, func() (string, error) {

		pods := k8s.ListPods(t, stage.Options, metav1.ListOptions{LabelSelector: "sslproxy=" + stage.Conf.Name})

		if len(pods) == 0 {
			return "fail", fmt.Errorf("waiting for at least one pod")
		}

		pod := pods[0]

		if pod.Annotations == nil {
			return "fail", fmt.Errorf("pod annotations are nil")
		}

		if pod.Annotations["updated"] == "" {
			return "fail", fmt.Errorf("pod updated annotation not set")
		}

		// at this point we assume that  update occured
		return "pass", nil
	})

}

/////////////////////////////////////////////
// STAGE
/////////////////////////////////////////////

type Stage struct {
	Conf       *TheConf
	Templates  *TemplatesStruct
	Options    *k8s.KubectlOptions
	RunOptions *RunOptions
}

func createStage() *Stage {
	conf := loadConf()
	stage := Stage{
		Conf:      conf,
		Templates: fromAll(conf),
		Options:   k8s.NewKubectlOptions("", "", conf.Namespace),
	}
	return &stage
}

func (stage *Stage) logConf() {
	fmt.Printf("Conf.Name: %v\n", stage.Conf.Name)
	fmt.Printf("Conf.IsFullTest: %v\n", stage.Conf.IsFullTest)
	fmt.Printf("Conf.LoadBalancerIP: %v\n", stage.Conf.LoadBalancerIP)
	fmt.Printf("Conf.StageTeardown: %v\n\n", stage.Conf.StageTeardown)
}

func (stage *Stage) unStage(t *testing.T) {
	defer k8s.KubectlDeleteFromString(t, stage.Options, stage.Templates.ServiceTemplate)
	defer k8s.KubectlDeleteFromString(t, stage.Options, stage.Templates.DeploymentTemplate)
	defer k8s.KubectlDeleteFromString(t, stage.Options, stage.Templates.SslProxyTemplate)
}

/**
 * here we stage the environment which involves setting up the SSLProxy
 */
func (stage *Stage) stage(t *testing.T) {

	fmt.Printf("***> stage.Conf.StageTeardown %v <***\n", stage.Conf.StageTeardown)
	if stage.Conf.StageTeardown {
		stage.unStage(t)
	}

	k8s.KubectlApplyFromString(t, stage.Options, stage.Templates.ServiceTemplate)
	k8s.KubectlApplyFromString(t, stage.Options, stage.Templates.DeploymentTemplate)

	// before we create an SSLProxy the service it points to must be available
	// otherwise NGINX will crash
	k8s.WaitUntilServiceAvailable(t, stage.Options, stage.Conf.Name, 60, time.Second*5)

	k8s.KubectlApplyFromString(t, stage.Options, stage.Templates.SslProxyTemplate)

	k8s.WaitUntilNumPodsCreated(t, stage.Options, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=" + stage.Conf.Name}, 1, 60, time.Second*10)
	k8s.WaitUntilNumPodsCreated(t, stage.Options, metav1.ListOptions{LabelSelector: "sslproxy=" + stage.Conf.Name}, 1, 60, time.Second*10)

	// wait for services that the operator should create
	k8s.WaitUntilServiceAvailable(t, stage.Options, stage.Conf.Name+"-nginx-sslproxy", 30, time.Second*10)
	k8s.WaitUntilServiceAvailable(t, stage.Options, stage.Conf.Name+"-certbot-service", 30, time.Second*10)
}

func (stage *Stage) setRunOptions(t *testing.T, runOptions *RunOptions) {
	stage.RunOptions = runOptions
	k8s.KubectlApplyFromString(t, stage.Options, from(OperatorOptionsConfigMap, runOptions))
}

/////////////////////////////////////////////
// stage tests
/////////////////////////////////////////////

func (stage *Stage) testExpectedIP(t *testing.T) {

	service := k8s.GetService(t, stage.Options, stage.Conf.Name+"-nginx-sslproxy")
	ipAddress := k8s.GetServiceEndpoint(t, stage.Options, service, 80)

	if ipAddress != stage.Conf.LoadBalancerIP+":80" {
		t.Errorf("ip address != loadBalancerIP expected: %v found: %v", stage.Conf.LoadBalancerIP, ipAddress)
		return
	}
}

func (stage *Stage) testExpectedSslProxyDefaultConf(t *testing.T) {
	pods := k8s.ListPods(t, stage.Options, metav1.ListOptions{LabelSelector: "sslproxy=" + stage.Conf.Name})
	pod := pods[0]
	k8s.WaitUntilPodAvailable(t, stage.Options, pod.Name, 4, time.Second*15)
	defaultConfig, err := k8s.RunKubectlAndGetOutputE(t, stage.Options, "exec", pod.Name, "--", "cat", "/etc/recert/conf/default.conf")
	if err != nil {
		t.Error(err, "expected defaultConfig to be located at /etc/recert/conf/default.conf")
	}
	if !strings.Contains(defaultConfig, "NGINX TEMPLATE FROM RECERT OPERATOR") {
		t.Error("expected header text in default.conf file not found: 'NGINX TEMPLATE FROM RECERT OPERATOR'")
	}
}

func (stage *Stage) testProxyWorking(t *testing.T) {
	service := k8s.GetService(t, stage.Options, stage.Conf.Name+"-nginx-sslproxy")
	ipAddress := k8s.GetServiceEndpoint(t, stage.Options, service, 80)
	http_helper.HttpGetWithRetryWithCustomValidation(t, "http://"+ipAddress, nil, 30, 5*time.Second, func(code int, body string) bool {
		return code == 200 && strings.Contains(body, "Hello World")
	})
}

func (stage *Stage) verifyState(t *testing.T, desc, state string, retries int, timeout time.Duration) {
	retry.DoWithRetry(t, desc, retries, timeout, func() (string, error) {
		output, err := k8s.RunKubectlAndGetOutputE(t, stage.Options, "get", "cert", stage.Conf.Name, "-o", "yaml")
		if err != nil {
			return "fail", err
		}

		var stub CertStub
		yaml.Unmarshal([]byte(output), &stub)

		if stub.Status.State != state {
			return "fail", fmt.Errorf("state is : %v", stub.Status.State)
		}

		return "pass", nil
	})
}

/////////////////////////////////////////////
// data structures
/////////////////////////////////////////////

// TheConf holds all the test configuration information which can be saved in a file conf.yaml
// The information in TheConf holds some information that can't be hard coded in the test framework
// like for instance the static ip Address and domain that will be tested
type TheConf struct {
	IsFullTest     bool              `yaml:"isFullTest,omitempty"`
	LoadBalancerIP string            `yaml:"loadBalancerIP,omitempty"`
	Domain         string            `yaml:"domain,omitempty"`
	Name           string            `yaml:"name"`
	Namespace      string            `yaml:"namespace"`
	StageTeardown  bool              `yaml:"stageTeardown,omitempty"`
	Images         map[string]string `yaml:"images,omitempty"`
}

// RunOptions options that modify how the operator will behave for an individual test
// typically expressed through the operator options ConfigMap
type RunOptions struct {
	CertRunMode string
}

type CertStub struct {
	Status CertStatus `yaml:"status,omitempty"`
}

type CertStatus struct {
	State       string `yaml:"state"`
	LastUpdated string `yaml:"lastUpdated"`
}

/////////////////////////////////////////////
// utility methods
/////////////////////////////////////////////

func loadConf() *TheConf {

	var conf TheConf
	{
		filename, _ := filepath.Abs("conf.yaml")
		yamlFile, err := ioutil.ReadFile(filename)

		err2 := yaml.Unmarshal(yamlFile, &conf)

		if err != nil || err2 != nil {
			println("using default conf.")
			conf.IsFullTest = false
			conf.Namespace = "recert-test-namespace"
			conf.Name = "test"
			conf.StageTeardown = true
		} else {
			println("loading conf from conf.yaml")
		}

	}

	// requires that `make build` be run before this test
	{
		filename, _ := filepath.Abs("../../../out/images-manifest.yaml")
		yamlFile, err := ioutil.ReadFile(filename)

		if err != nil {
			panic("cannot load \"../../../out/images-manifest.yaml\" which must be built by \"make build\" at the root of this repository before this test can be run")
		}
		err = yaml.Unmarshal(yamlFile, &conf.Images)
		if err != nil {
			panic("cannot unmarshal \"../../../out/images-manifest.yaml\" which must be built by \"make build\" at the root of this repository before this test can be run")
		}
	}

	return &conf
}
