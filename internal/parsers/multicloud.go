package parsers

import (
	"strings"

	"terraviz/internal/models"
)

// multiCloudTypeMap maps Google Cloud and Azure Terraform resource types to
// internal types. It is kept separate from terraformTypeMap so the AWS table
// stays readable; mapTerraformType consults both.
var multiCloudTypeMap = map[string]models.ResourceType{
	// Google Cloud
	"google_compute_instance":         models.ResourceTypeGCPInstance,
	"google_compute_network":          models.ResourceTypeGCPNetwork,
	"google_compute_subnetwork":       models.ResourceTypeGCPSubnetwork,
	"google_compute_firewall":         models.ResourceTypeGCPFirewall,
	"google_compute_forwarding_rule":  models.ResourceTypeGCPForwardingRule,
	"google_compute_address":          models.ResourceTypeGCPAddress,
	"google_storage_bucket":           models.ResourceTypeGCPStorageBucket,
	"google_sql_database_instance":    models.ResourceTypeGCPSQLInstance,
	"google_cloudfunctions_function":  models.ResourceTypeGCPFunction,
	"google_cloudfunctions2_function": models.ResourceTypeGCPFunction,
	"google_container_cluster":        models.ResourceTypeGCPContainerCluster,
	"google_pubsub_topic":             models.ResourceTypeGCPPubSubTopic,
	"google_pubsub_subscription":      models.ResourceTypeGCPPubSubSub,
	"google_dns_managed_zone":         models.ResourceTypeGCPDNSZone,
	"google_kms_crypto_key":           models.ResourceTypeGCPKMSKey,

	// Microsoft Azure
	"azurerm_virtual_machine":         models.ResourceTypeAzureVM,
	"azurerm_linux_virtual_machine":   models.ResourceTypeAzureVM,
	"azurerm_windows_virtual_machine": models.ResourceTypeAzureVM,
	"azurerm_virtual_network":         models.ResourceTypeAzureVNet,
	"azurerm_subnet":                  models.ResourceTypeAzureSubnet,
	"azurerm_network_security_group":  models.ResourceTypeAzureNSG,
	"azurerm_public_ip":               models.ResourceTypeAzurePublicIP,
	"azurerm_lb":                      models.ResourceTypeAzureLB,
	"azurerm_load_balancer":           models.ResourceTypeAzureLB,
	"azurerm_storage_account":         models.ResourceTypeAzureStorageAccount,
	"azurerm_mssql_database":          models.ResourceTypeAzureSQLDatabase,
	"azurerm_sql_database":            models.ResourceTypeAzureSQLDatabase,
	"azurerm_function_app":            models.ResourceTypeAzureFunctionApp,
	"azurerm_linux_function_app":      models.ResourceTypeAzureFunctionApp,
	"azurerm_windows_function_app":    models.ResourceTypeAzureFunctionApp,
	"azurerm_kubernetes_cluster":      models.ResourceTypeAzureAKSCluster,
	"azurerm_servicebus_queue":        models.ResourceTypeAzureServiceBusQueue,
	"azurerm_servicebus_namespace":    models.ResourceTypeAzureServiceBusQueue,
	"azurerm_eventhub":                models.ResourceTypeAzureEventHub,
	"azurerm_key_vault":               models.ResourceTypeAzureKeyVault,
	"azurerm_cosmosdb_account":        models.ResourceTypeAzureCosmosDB,
	"azurerm_resource_group":          models.ResourceTypeAzureResourceGroup,
}

// isSupportedProvider reports whether resources from this provider are kept in
// the diagram. Unmapped types are retained generically for all three so a
// multi-cloud state is never silently truncated.
func isSupportedProvider(providerName string) bool {
	name := unwrapProviderName(providerName)
	name = strings.TrimPrefix(name, "registry.terraform.io/")
	base := name
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		base = name[idx+1:]
	}
	switch base {
	case "aws", "google", "azurerm", "azure":
		return true
	default:
		return false
	}
}

// providerForType infers the canonical provider name from a Terraform type.
// Terraform's raw state sometimes omits or shortens the provider address, so
// the type prefix is a useful fallback.
func providerForType(tfType, fallback string) string {
	switch {
	case strings.HasPrefix(tfType, "google_"):
		return "gcp"
	case strings.HasPrefix(tfType, "azurerm_"):
		return "azure"
	default:
		return fallback
	}
}
