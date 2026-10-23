package parsers

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"terraviz/internal/models"
)

// maxReferenceDepth limits how deep the walk through the properties of a
// resource descends.
const maxReferenceDepth = 16

// DetectMissingReferences returns the resources that the given resources
// reference but do not contain. A reference is a string value that looks like
// an AWS ARN or like the identifier of a resource type TerraViz models.
//
// The placeholder nodes of the external reference specs stand for the very
// resources that are reported here, so they neither satisfy a reference nor
// produce one.
func DetectMissingReferences(resources []models.Resource) []models.ReferencedMissing {
	present := presentIdentifiers(resources)
	found := make(map[missingKey]*models.ReferencedMissing)

	for i := range resources {
		resource := &resources[i]
		if isExternalReference(resource) {
			continue
		}
		for _, value := range referenceStrings(resource.Properties) {
			ref, ok := classifyReference(value)
			if !ok {
				continue
			}
			if present[ref.value] {
				// The reference is part of the state, or it is the
				// identifier of the resource that holds it.
				continue
			}
			key := missingKey{resourceType: ref.resourceType, value: ref.value}
			entry, exists := found[key]
			if !exists {
				entry = &models.ReferencedMissing{Type: ref.resourceType}
				if ref.arn {
					entry.ARN = ref.value
				} else {
					entry.ID = ref.value
				}
				found[key] = entry
			}
			if !containsString(entry.ReferencedBy, resource.ID) {
				entry.ReferencedBy = append(entry.ReferencedBy, resource.ID)
			}
		}
	}

	missing := make([]models.ReferencedMissing, 0, len(found))
	for _, entry := range found {
		sort.Strings(entry.ReferencedBy)
		entry.Description = fmt.Sprintf("Referenced by %s but not present in the state file",
			strings.Join(entry.ReferencedBy, ", "))
		missing = append(missing, *entry)
	}
	sort.Slice(missing, func(i, j int) bool {
		if missing[i].Type != missing[j].Type {
			return missing[i].Type < missing[j].Type
		}
		if missing[i].ARN != missing[j].ARN {
			return missing[i].ARN < missing[j].ARN
		}
		return missing[i].ID < missing[j].ID
	})

	return missing
}

// missingKey identifies a missing resource in the detection map.
type missingKey struct {
	resourceType models.ResourceType
	value        string
}

// isExternalReference reports whether a resource is a placeholder for a
// reference that points outside the state.
func isExternalReference(resource *models.Resource) bool {
	return resource.Source == externalReferenceSource
}

// presentIdentifiers returns the identifiers of the resources that are part of
// the state. The placeholder nodes of the external reference specs are left
// out: they stand for the resources that are reported as missing.
func presentIdentifiers(resources []models.Resource) map[string]bool {
	present := make(map[string]bool)
	for i := range resources {
		resource := &resources[i]
		if isExternalReference(resource) {
			continue
		}
		for _, key := range []string{"id", "arn"} {
			if value, ok := resource.Properties[key].(string); ok && value != "" {
				present[value] = true
			}
		}
	}
	return present
}

// referenceStrings returns every string value of the properties of a
// resource, including the values of nested lists, nested maps and JSON
// encoded strings such as a redrive policy.
func referenceStrings(properties map[string]interface{}) []string {
	var values []string
	seen := make(map[string]bool)

	var walk func(value interface{}, depth int)
	walk = func(value interface{}, depth int) {
		if depth > maxReferenceDepth {
			return
		}
		switch typed := value.(type) {
		case string:
			if typed == "" || seen[typed] {
				return
			}
			seen[typed] = true
			values = append(values, typed)
			if decoded, ok := jsonObject(typed); ok {
				walk(decoded, depth+1)
			}
		case []interface{}:
			for _, item := range typed {
				walk(item, depth+1)
			}
		case map[string]interface{}:
			for _, item := range typed {
				walk(item, depth+1)
			}
		}
	}

	for key, value := range properties {
		if key == "tags" {
			// Tags are labels, not references.
			continue
		}
		walk(value, 0)
	}

	return values
}

// jsonObject decodes a JSON encoded attribute into a value that can be
// walked. It reports false for values that are not a JSON object or array.
func jsonObject(value string) (interface{}, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
		return nil, false
	}
	decoded := decodeJSONValue(value)
	switch decoded.(type) {
	case map[string]interface{}, []interface{}:
		return decoded, true
	default:
		return nil, false
	}
}

// reference is a string value that points at a resource.
type reference struct {
	value        string
	resourceType models.ResourceType
	arn          bool
}

// classifyReference decides whether a value references a resource and which
// type of resource it references.
func classifyReference(value string) (reference, bool) {
	if strings.ContainsAny(value, "*?") {
		// A wildcard names a pattern, not a single resource. The objects of
		// an S3 bucket are the one case where the pattern still names a
		// resource that can be reported.
		trimmed, ok := s3ObjectARN(value)
		if !ok {
			return reference{}, false
		}
		value = trimmed
	}

	if strings.HasPrefix(value, "arn:") {
		parts := parseARN(value)
		if parts == nil {
			return reference{}, false
		}
		return reference{value: value, resourceType: arnResourceType(parts), arn: true}, true
	}

	for _, idPattern := range idPatterns {
		if idPattern.pattern.MatchString(value) {
			return reference{value: value, resourceType: idPattern.resourceType}, true
		}
	}

	return reference{}, false
}

// s3ObjectARN returns the bucket ARN of an S3 object pattern such as
// "arn:aws:s3:::my-bucket/*".
func s3ObjectARN(value string) (string, bool) {
	if !strings.HasPrefix(value, "arn:aws:s3:::") || !strings.HasSuffix(value, "/*") {
		return "", false
	}
	return strings.TrimSuffix(value, "/*"), true
}

// arnParts holds the parts of an AWS ARN that TerraViz uses.
type arnParts struct {
	service  string
	resource string
}

// parseARN splits an ARN into its parts. It returns nil for anything that is
// not an AWS ARN.
func parseARN(value string) *arnParts {
	fields := strings.SplitN(value, ":", 6)
	if len(fields) < 6 || fields[0] != "arn" || !strings.HasPrefix(fields[1], "aws") {
		return nil
	}
	return &arnParts{service: fields[2], resource: fields[5]}
}

// arnResourceType maps the service and the type of an ARN onto a resource
// type. ARNs of a service that TerraViz does not model keep the type of the
// ARN as their label.
func arnResourceType(parts *arnParts) models.ResourceType {
	arnType := arnTypeSegment(parts.resource)
	if resourceType, ok := arnResourceTypes[parts.service+":"+arnType]; ok {
		return resourceType
	}
	if arnType == "" {
		if resourceType, ok := arnServiceTypes[parts.service]; ok {
			return resourceType
		}
		return models.ResourceType(parts.service)
	}
	return models.ResourceType(arnType)
}

// arnTypeSegment returns the type of the resource part of an ARN, for example
// "targetgroup" in "targetgroup/app/prod/1234". ARNs of services such as SQS
// or SNS carry no type and return an empty string.
func arnTypeSegment(resourcePart string) string {
	part := strings.TrimPrefix(resourcePart, "/")
	if !strings.ContainsAny(part, "/:") {
		return ""
	}

	segments := strings.Split(part, "/")
	segment := segments[0]
	if (segment == "regional" || segment == "global") && len(segments) > 1 {
		segment = segments[1]
	}
	if idx := strings.Index(segment, ":"); idx >= 0 {
		segment = segment[:idx]
	}
	return segment
}

// arnResourceTypes maps the service and the type of an ARN onto the resource
// types of TerraViz.
var arnResourceTypes = map[string]models.ResourceType{
	"acm:certificate":                    models.ResourceTypeACMCertificate,
	"apigateway:restapis":                models.ResourceTypeAPIGateway,
	"cloudfront:distribution":            models.ResourceTypeCloudFront,
	"ec2:instance":                       models.ResourceTypeEC2Instance,
	"ec2:security-group":                 models.ResourceTypeSecurityGroup,
	"ec2:subnet":                         models.ResourceTypeSubnet,
	"ec2:vpc":                            models.ResourceTypeVPC,
	"ecr:repository":                     models.ResourceTypeECRRepository,
	"ecs:cluster":                        models.ResourceTypeECSCluster,
	"ecs:service":                        models.ResourceTypeECSService,
	"ecs:task-definition":                models.ResourceTypeECSTaskDefinition,
	"elasticloadbalancing:listener":      models.ResourceTypeLBListener,
	"elasticloadbalancing:listener-rule": models.ResourceTypeLBListenerRule,
	"elasticloadbalancing:loadbalancer":  models.ResourceTypeLoadBalancer,
	"elasticloadbalancing:targetgroup":   models.ResourceTypeLBTargetGroup,
	"iam:policy":                         models.ResourceTypeIAMPolicy,
	"iam:role":                           models.ResourceTypeIAMRole,
	"iam:user":                           models.ResourceTypeIAMUser,
	"kms:key":                            models.ResourceTypeKMSKey,
	"lambda:function":                    models.ResourceTypeLambdaFunction,
	"logs:log-group":                     models.ResourceTypeCloudWatch,
	"rds:db":                             models.ResourceTypeRDSInstance,
	"route53:hostedzone":                 models.ResourceTypeRoute53Zone,
	"secretsmanager:secret":              models.ResourceTypeSecretsManager,
	"servicediscovery:namespace":         models.ResourceTypeServiceDiscoveryNamespace,
	"servicediscovery:service":           models.ResourceTypeServiceDiscoveryService,
	"wafv2:ipset":                        models.ResourceTypeWAFIPSet,
	"wafv2:webacl":                       models.ResourceTypeWAFWebACL,
}

// arnServiceTypes maps the services whose ARNs carry no type onto a resource
// type.
var arnServiceTypes = map[string]models.ResourceType{
	"s3":  models.ResourceTypeS3Bucket,
	"sns": models.ResourceTypeSNSTopic,
	"sqs": models.ResourceTypeSQSQueue,
}

// idPatterns recognises the identifiers of the resource types TerraViz models,
// so that a reference to an identifier that is not in the state is reported.
var idPatterns = []struct {
	resourceType models.ResourceType
	pattern      *regexp.Regexp
}{
	{models.ResourceTypeEC2Instance, regexp.MustCompile(`^i-[0-9a-z]{6,32}$`)},
	{models.ResourceTypeSecurityGroup, regexp.MustCompile(`^sg-[0-9a-z]{6,32}$`)},
	{models.ResourceTypeSubnet, regexp.MustCompile(`^subnet-[0-9a-z]{6,32}$`)},
	{models.ResourceTypeVPC, regexp.MustCompile(`^vpc-[0-9a-z]{6,32}$`)},
	{models.ResourceTypeKMSKey, regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)},
	{models.ResourceTypeRoute53Zone, regexp.MustCompile(`^Z[A-Z0-9]{3,32}$`)},
	{models.ResourceTypeAPIGateway, regexp.MustCompile(`^api-[0-9a-z]{10}$`)},
}
