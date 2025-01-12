package main

import (
	"bytes"
	"encoding/json"
	"encoding/base64"
	"fmt"
	"os"
	"net/http"

	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/rest"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
)

var GroupName = os.Getenv("GROUP_NAME")

type TriggerInstance struct {
    WebhookID string `json:"webhook_id"`
    String1   string `json:"string1"`
    String2   string `json:"string2"`
    String3   string `json:"string3"`
    String4   string `json:"string4"`
}

type TriggerData struct {
    TriggerType         string            `json:"trigger_type"`
    TriggerInstanceList []TriggerInstance `json:"trigger_instance_list"`
}

func basicAuthHeader(username, password string) string {
	auth := username + ":" + password
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(auth))
	return "Basic " + encodedAuth
}

func (c *PrismCentralWebhookProviderSolver) sendRequest(data []byte, url string, username string, password string) error {

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", basicAuthHeader(username, password))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received non-OK response code: %d", resp.StatusCode)
	}

	return nil
}


func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	// This will register our custom DNS provider with the webhook serving
	// library, making it available as an API under the provided GroupName.
	// You can register multiple DNS provider implementations with a single
	// webhook, where the Name() method will be used to disambiguate between
	// the different implementations.
	cmd.RunWebhookServer(GroupName,
		&PrismCentralWebhookProviderSolver{},
	)
}

// PrismCentralWebhookProviderSolver implements the provider-specific logic needed to
// 'present' an ACME challenge TXT record for your own DNS provider.
// To do so, it must implement the `github.com/cert-manager/cert-manager/pkg/acme/webhook.Solver`
// interface.
type PrismCentralWebhookProviderSolver struct {
	// If a Kubernetes 'clientset' is needed, you must:
	// 1. uncomment the additional `client` field in this structure below
	// 2. uncomment the "k8s.io/client-go/kubernetes" import at the top of the file
	// 3. uncomment the relevant code in the Initialize method below
	// 4. ensure your webhook's service account has the required RBAC role
	//    assigned to it for interacting with the Kubernetes APIs you need.
	// client     *kubernetes.Clientset
}

// customDNSProviderConfig is a structure that is used to decode into when
// solving a DNS01 challenge.
// This information is provided by cert-manager, and may be a reference to
// additional configuration that's needed to solve the challenge for this
// particular certificate or issuer.
// This typically includes references to Secret resources containing DNS
// provider credentials, in cases where a 'multi-tenant' DNS solver is being
// created.
// If you do *not* require per-issuer or per-certificate configuration to be
// provided to your webhook, you can skip decoding altogether in favour of
// using CLI flags or similar to provide configuration.
// You should not include sensitive information here. If credentials need to
// be used by your provider here, you should reference a Kubernetes Secret
// resource and fetch these credentials using a Kubernetes clientset.



type customDNSProviderConfig struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	APIEndpoint string `json:"apiEndpoint"`
	WebhookID string `json:"webhookID"`

	// Change the two fields below according to the format of the configuration
	// to be decoded.
	// These fields will be set by users in the
	// `issuer.spec.acme.dns01.providers.webhook.config` field.

	//Email           string `json:"email"`

	//APIKeySecretRef v1alpha1.SecretKeySelector `json:"apiKeySecretRef"`
}

// Name is used as the name for this DNS solver when referencing it on the ACME
// Issuer resource.
// This should be unique **within the group name**, i.e. you can have two
// solvers configured with the same Name() **so long as they do not co-exist
// within a single webhook deployment**.
// For example, `cloudflare` may be used as the name of a solver.
func (c *PrismCentralWebhookProviderSolver) Name() string {
	return "prismcentral-solver"
}

// Present is responsible for actually presenting the DNS record with the
// DNS provider.
// This method should tolerate being called multiple times with the same value.
// cert-manager itself will later perform a self check to ensure that the
// solver has correctly configured the DNS provider.
func (c *PrismCentralWebhookProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}

	// Debug output for loaded configuration
	fmt.Printf("Loaded configuration: %+v\n", cfg)

    triggerData := TriggerData{
        TriggerType: "incoming_webhook_trigger",
        TriggerInstanceList: []TriggerInstance{
            {
                WebhookID: cfg.WebhookID,
                String1:   "Add",
                String2:   ch.Key,
                String3:   ch.ResolvedFQDN,
                String4:   ch.ResolvedZone,
            },
        },
    }


	// Marshal webhookConfig to JSON for the request body
	jsonData, err := json.Marshal(triggerData)
	if err != nil {
		return fmt.Errorf("error marshaling JSON: %v", err)
	}

	// Debug output for data that will be sent in the request
	fmt.Printf("Data for DNS challenge: %+v\n", string(jsonData))

	err = c.sendRequest(jsonData,cfg.APIEndpoint, cfg.Username, cfg.Password)
	if err != nil {
		// Debug output in case of an error during the request
		fmt.Printf("Error sending request: %v\n", err)
		return err
	}

	// Debug output indicating success
	fmt.Println("Successfully presented DNS challenge")
	return nil
}

// CleanUp should delete the relevant TXT record from the DNS provider console.
// If multiple TXT records exist with the same record name (e.g.
// _acme-challenge.example.com) then **only** the record with the same `key`
// value provided on the ChallengeRequest should be cleaned up.
// This is in order to facilitate multiple DNS validations for the same domain
// concurrently.
func (c *PrismCentralWebhookProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}

    triggerData := TriggerData{
        TriggerType: "incoming_webhook_trigger",
        TriggerInstanceList: []TriggerInstance{
            {
                WebhookID: "90836c84-38ce-456e-b595-7bdab0bdffb3",
                String1:   "Add",
                String2:   ch.Key,
                String3:   ch.ResolvedFQDN,
                String4:   ch.ResolvedZone,
            },
        },
    }

	// Marshal webhookConfig to JSON for the request body
	jsonData, err := json.Marshal(triggerData)
	if err != nil {
		return fmt.Errorf("error marshaling JSON: %v", err)
	}

	// Debug output for data that will be sent in the request
	fmt.Printf("Data for DNS challenge: %+v\n", string(jsonData))

	return c.sendRequest(jsonData, cfg.APIEndpoint, cfg.Username, cfg.Password) 
}


// Initialize will be called when the webhook first starts.
// This method can be used to instantiate the webhook, i.e. initialising
// connections or warming up caches.
// Typically, the kubeClientConfig parameter is used to build a Kubernetes
// client that can be used to fetch resources from the Kubernetes API, e.g.
// Secret resources containing credentials used to authenticate with DNS
// provider accounts.
// The stopCh can be used to handle early termination of the webhook, in cases
// where a SIGTERM or similar signal is sent to the webhook process.
func (c *PrismCentralWebhookProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	// cl, err := kubernetes.NewForConfig(kubeClientConfig)
	// if err != nil {
	// 	return err
	// }
	
	// c.client = cl

	return nil
}

// loadConfig is a small helper function that decodes JSON configuration into
// the typed config struct.
func loadConfig(cfgJSON *extapi.JSON) (customDNSProviderConfig, error) {
	cfg := customDNSProviderConfig{}
	// handle the 'base case' where no configuration has been provided
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding solver config: %v", err)
	}

	return cfg, nil
}
