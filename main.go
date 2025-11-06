package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
	"io"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"net/http"
	"os"
	"strings"
)

var GroupName = os.Getenv("GROUP_NAME")

type Config struct {
	ApiUrl, ApiKey, ApiSecret string
}

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	cmd.RunWebhookServer(GroupName,
		&regeryDNSProviderSolver{},
	)
}

type regeryDNSProviderSolver struct {
	client *kubernetes.Clientset
}

type regeryDNSProviderConfig struct {
	SecretRef string `json:"secretName"`
}

func (c *regeryDNSProviderSolver) Name() string {
	return "regery"
}

func stringFromSecretData(secretData map[string][]byte, key string) (string, error) {
	data, ok := secretData[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret data", key)
	}
	return string(data), nil
}

func (c *regeryDNSProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {

	klog.V(6).Infof("call function Present: namespace=%s, zone=%s, fqdn=%s",
		ch.ResourceNamespace, ch.ResolvedZone, ch.ResolvedFQDN)

	config, err := clientConfig(c, ch)

	if err != nil {
		return fmt.Errorf("unable to get secret `%s`; %v", ch.ResourceNamespace, err)
	}

	domain := strings.TrimSuffix(ch.ResolvedZone, ".")
	url := config.ApiUrl + "/domains/" + domain + "/records"
	name := extractRecordName(ch.ResolvedFQDN, domain)

	payload := map[string]interface{}{
		"records": []map[string]string{
			{
				"type":  "TXT",
				"name":  name,
				"value": ch.Key,
				"ttl":   "60",
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %v", err)
	}

	_, err = callDnsApi(url, "POST", bytes.NewReader(body), config)
	if err != nil {
		klog.Error(err)
		return err
	}

	klog.Infof("Presented TXT record for %v", ch.ResolvedFQDN)

	return nil
}

func clientConfig(c *regeryDNSProviderSolver, ch *v1alpha1.ChallengeRequest) (Config, error) {
	var config Config

	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return config, err
	}

	config.ApiUrl = "https://api.regery.com/v1"

	secretName := cfg.SecretRef
	sec, err := c.client.CoreV1().Secrets(ch.ResourceNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})

	if err != nil {
		return config, fmt.Errorf("unable to get secret `%s/%s`; %v", secretName, ch.ResourceNamespace, err)
	}

	apiKey, err := stringFromSecretData(sec.Data, "api-key")

	if err != nil {
		return config, fmt.Errorf("unable to get api-key from secret `%s/%s`; %v", secretName, ch.ResourceNamespace, err)
	}

	config.ApiKey = apiKey

	apiSecret, err := stringFromSecretData(sec.Data, "api-secret")

	if err != nil {
		return config, fmt.Errorf("unable to get api-secret from secret `%s/%s`; %v", secretName, ch.ResourceNamespace, err)
	}

	config.ApiSecret = apiSecret

	return config, nil
}

func callDnsApi(url, method string, body io.Reader, config Config) ([]byte, error) {
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return []byte{}, fmt.Errorf("unable to execute request %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", config.ApiKey+":"+config.ApiSecret)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		err := resp.Body.Close()
		if err != nil {
			klog.Fatal(err)
		}
	}()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		return respBody, nil
	}

	text := "Error calling API status:" + resp.Status + " url: " + url + " method: " + method
	klog.Error(text)
	return nil, errors.New(text)
}

func extractRecordName(fqdn string, zone string) (string) {
	// Trim trailing dots, if any
	fqdn = strings.TrimSuffix(fqdn, ".")
	zone = strings.TrimSuffix(zone, ".")

	// Remove the zone from the FQDN, if present
	if strings.HasSuffix(fqdn, zone) {
		label := fqdn[:len(fqdn)-len(zone)]
		label = strings.TrimSuffix(label, ".")
		return label
	}

	return fqdn
}

func (c *regeryDNSProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	config, err := clientConfig(c, ch)
	if err != nil {
		return fmt.Errorf("unable to get secret `%s`; %v", ch.ResourceNamespace, err)
	}

	domain := strings.TrimSuffix(ch.ResolvedZone, ".")
	recordName := ch.ResolvedFQDN
	recordName = extractRecordName(recordName, domain)

	// Build JSON payload for the DELETE request
	payload := map[string]interface{}{
		"records": []map[string]string{
			{
				"type":  "TXT",
				"name":  recordName,
				"value": ch.Key,
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal delete payload: %v", err)
	}

	url := config.ApiUrl + "/domains/" + domain + "/records"
	_, err = callDnsApi(url, "DELETE", bytes.NewReader(body), config)
	if err != nil {
		klog.Error(err)
		return err
	}
	klog.Infof("Deleted TXT record for %s", ch.ResolvedFQDN)
	return nil
}

func (c *regeryDNSProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {

	k8sClient, err := kubernetes.NewForConfig(kubeClientConfig)
	klog.V(6).Infof("Input variable stopCh is %d length", len(stopCh))
	if err != nil {
		return err
	}

	c.client = k8sClient

	return nil
}

func loadConfig(cfgJSON *extapi.JSON) (regeryDNSProviderConfig, error) {
	cfg := regeryDNSProviderConfig{}
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %v", err)
	}

	return cfg, nil
}
