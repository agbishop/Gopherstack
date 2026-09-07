package main

import (
	"maps"
	"slices"
	"strings"
)

// categoryGroup is one AWS-style category heading in the root README service
// table, together with the service slugs that belong under it.
type categoryGroup struct {
	Name  string
	Slugs []string
}

// The five Azure service slugs, as constants rather than repeated string
// literals -- each one is referenced from categoryGroups, the display-name
// overrides, and categories_test.go, which would otherwise trip goconst's
// repeated-literal check.
const (
	slugAzureBlob       = "azureblob"
	slugAzureQueue      = "azurequeue"
	slugAzureTable      = "azuretable"
	slugAzureServiceBus = "azureservicebus"
	slugCosmosDB        = "cosmosdb"
)

// categoryGroups is the curated slug -> category assignment for every
// service under services/ (158 entries, including qldb/qldbsession which are
// rendered as "Removed" rows). Every slug must appear in exactly one group;
// see the cross-check in the gendocs README/report for how this was derived.
//
// Kept as a function (not a package-level var) per the repo's
// gochecknoglobals convention — build-fresh-per-call is cheap here since
// gendocs runs once as a CLI, not in a hot path.
func categoryGroups() []categoryGroup {
	return []categoryGroup{
		{"Compute", []string{
			"apprunner", "autoscaling", "batch", "ec2", "elasticbeanstalk", "lambda",
		}},
		{"Containers", []string{
			"ecr", "ecs", "eks",
		}},
		{"Storage", []string{
			"backup", "dlm", "efs", "fsx", "glacier", "s3", "s3control", "s3tables",
		}},
		{"Database", []string{
			"dax", "docdb", "dynamodb", "dynamodbstreams", "elasticache", "memorydb",
			"neptune", "qldb", "qldbsession", "rds", "rdsdata", "redshift",
			"redshiftdata", "timestreamquery", "timestreamwrite",
		}},
		{"Networking & Content Delivery", []string{
			"apigateway", "apigatewaymanagementapi", "apigatewayv2", "appmesh",
			"cloudfront", "elb", "elbv2", "networkmonitor", "route53",
			"route53resolver", "servicediscovery", "vpclattice",
		}},
		{"Messaging & Integration", []string{
			"appsync", "eventbridge", "mq", "pinpoint", "pipes", "scheduler", "ses",
			"sesv2", "sns", "sqs", "stepfunctions", "swf", "workmail",
		}},
		{"Analytics", []string{
			"athena", "cleanrooms", "databrew", "elasticsearch", "emr",
			"emrserverless", "firehose", "glue", "kafka", "kinesis",
			"kinesisanalytics", "kinesisanalyticsv2", "lakeformation", "mwaa",
			"opensearch", "quicksight",
		}},
		{"Security", []string{
			"acm", "acmpca", "detective", "guardduty", "inspector2", "kms", "macie2",
			"secretsmanager", "securityhub", "shield", "verifiedpermissions", "waf",
			"wafv2",
		}},
		{"Identity & Access", []string{
			"accessanalyzer", "cognitoidentity", "cognitoidp", "directoryservice",
			"iam", "identitystore", "rolesanywhere", "ssoadmin", "sts",
		}},
		{"Management & Governance", []string{
			"account", "appconfig", "appconfigdata", "applicationautoscaling",
			"awsconfig", "ce", "cloudcontrol", "cloudformation", "cloudtrail",
			"cloudwatch", "cloudwatchlogs", "fis", "opsworks", "organizations",
			"ram", "resourcegroups", "resourcegroupstaggingapi", "ssm",
		}},
		{"Developer Tools", []string{
			"amplify", "codeartifact", "codebuild", "codecommit", "codeconnections",
			"codedeploy", "codepipeline", "codestarconnections", "serverlessrepo",
			"xray",
		}},
		{"Machine Learning", []string{
			"bedrock", "bedrockagent", "bedrockruntime", "comprehend", "forecast",
			"personalize", "polly", "rekognition", "sagemaker", "sagemakerruntime",
			"textract", "transcribe", "translate",
		}},
		{"Media", []string{
			"mediaconvert", "medialive", "mediapackage", "mediastore",
			"mediastoredata", "mediatailor",
		}},
		{"IoT", []string{
			"iot", "iotanalytics", "iotdataplane", "iotwireless",
		}},
		{"Migration & Transfer", []string{
			"datasync", "dms", "transfer",
		}},
		{"Azure", []string{
			slugAzureBlob, slugAzureQueue, slugAzureTable, slugAzureServiceBus, slugCosmosDB,
		}},
		{"Other", []string{
			"appstream", "managedblockchain", "omics", "support", "workspaces",
		}},
	}
}

// categoryFor returns the curated category for slug, falling back to "Other"
// for anything not in categoryGroups (should not happen for the 154 known
// services, but keeps the generator from panicking on a new/unknown one).
func categoryFor(slug string) string {
	for _, g := range categoryGroups() {
		if slices.Contains(g.Slugs, slug) {
			return g.Name
		}
	}

	return "Other"
}

// categoryOrder returns the category names in the fixed display order used
// for the root README table.
func categoryOrder() []string {
	groups := categoryGroups()
	order := make([]string, 0, len(groups))
	for _, g := range groups {
		order = append(order, g.Name)
	}

	return order
}

// displayNameOverrides is the curated slug -> friendly-name map for services
// whose title-cased slug would otherwise read poorly (acronyms, versioned
// APIs, multi-word run-together slugs, etc.). Anything not listed here falls
// back to titleCaseSlug. Split into four alphabetical batches
// (displayNamesA..D) purely to stay under funlen's per-function line limit.
func displayNameOverrides() map[string]string {
	m := make(map[string]string)
	maps.Copy(m, displayNamesA())
	maps.Copy(m, displayNamesB())
	maps.Copy(m, displayNamesC())
	maps.Copy(m, displayNamesD())

	return m
}

func displayNamesA() map[string]string {
	return map[string]string{
		"accessanalyzer":          "IAM Access Analyzer",
		"account":                 "Account",
		"acm":                     "ACM",
		"acmpca":                  "ACM PCA",
		"amplify":                 "Amplify",
		"apigateway":              "API Gateway",
		"apigatewaymanagementapi": "API Gateway Management API",
		"apigatewayv2":            "API Gateway v2",
		"appconfig":               "AppConfig",
		"appconfigdata":           "AppConfig Data",
		"applicationautoscaling":  "Application Auto Scaling",
		"appmesh":                 "App Mesh",
		"apprunner":               "App Runner",
		"appstream":               "AppStream 2.0",
		"appsync":                 "AppSync",
		"athena":                  "Athena",
		"autoscaling":             "Auto Scaling",
		"awsconfig":               "Config",
		slugAzureBlob:             "Azure Blob Storage",
		slugAzureQueue:            "Azure Queue Storage",
		slugAzureTable:            "Azure Table Storage",
		slugAzureServiceBus:       "Azure Service Bus",
		"backup":                  "Backup",
		"batch":                   "Batch",
		"bedrock":                 "Bedrock",
		"bedrockagent":            "Bedrock Agent",
		"bedrockruntime":          "Bedrock Runtime",
		"ce":                      "Cost Explorer",
		"cleanrooms":              "Clean Rooms",
		"cloudcontrol":            "Cloud Control API",
		"cloudformation":          "CloudFormation",
		"cloudfront":              "CloudFront",
		"cloudtrail":              "CloudTrail",
		"cloudwatch":              "CloudWatch",
		"cloudwatchlogs":          "CloudWatch Logs",
		"codeartifact":            "CodeArtifact",
		"codebuild":               "CodeBuild",
		"codecommit":              "CodeCommit",
		"codeconnections":         "CodeConnections",
		"codedeploy":              "CodeDeploy",
		"codepipeline":            "CodePipeline",
		"codestarconnections":     "CodeStar Connections",
		"cognitoidentity":         "Cognito Identity",
	}
}

func displayNamesB() map[string]string {
	return map[string]string{
		"cognitoidp":       "Cognito Identity Provider",
		"comprehend":       "Comprehend",
		slugCosmosDB:       "Azure Cosmos DB",
		"databrew":         "Glue DataBrew",
		"datasync":         "DataSync",
		"dax":              "DAX",
		"detective":        "Detective",
		"directoryservice": "Directory Service",
		"dlm":              "Data Lifecycle Manager",
		"dms":              "Database Migration Service",
		"docdb":            "DocumentDB",
		"dynamodb":         "DynamoDB",
		"dynamodbstreams":  "DynamoDB Streams",
		"ec2":              "EC2",
		"ecr":              "ECR",
		"ecs":              "ECS",
		"efs":              "EFS",
		"eks":              "EKS",
		"elasticache":      "ElastiCache",
		"elasticbeanstalk": "Elastic Beanstalk",
		"elasticsearch":    "Elasticsearch",
		"elb":              "ELB (Classic)",
		"elbv2":            "ELBv2",
		"emr":              "EMR",
		"emrserverless":    "EMR Serverless",
		"eventbridge":      "EventBridge",
		"firehose":         "Kinesis Data Firehose",
		"fis":              "Fault Injection Simulator",
		"forecast":         "Forecast",
		"fsx":              "FSx",
		"glacier":          "S3 Glacier",
		"glue":             "Glue",
		"guardduty":        "GuardDuty",
		"iam":              "IAM",
		"identitystore":    "Identity Store",
		"inspector2":       "Inspector",
		"iot":              "IoT Core",
		"iotanalytics":     "IoT Analytics",
		"iotdataplane":     "IoT Data Plane",
		"iotwireless":      "IoT Wireless",
	}
}

func displayNamesC() map[string]string {
	return map[string]string{
		"kafka":                    "Managed Streaming for Kafka",
		"kinesis":                  "Kinesis",
		"kinesisanalytics":         "Kinesis Analytics",
		"kinesisanalyticsv2":       "Kinesis Analytics v2",
		"kms":                      "KMS",
		"lakeformation":            "Lake Formation",
		"lambda":                   "Lambda",
		"macie2":                   "Macie",
		"managedblockchain":        "Managed Blockchain",
		"mediaconvert":             "MediaConvert",
		"medialive":                "MediaLive",
		"mediapackage":             "MediaPackage",
		"mediastore":               "MediaStore",
		"mediastoredata":           "MediaStore Data",
		"mediatailor":              "MediaTailor",
		"memorydb":                 "MemoryDB",
		"mq":                       "Amazon MQ",
		"mwaa":                     "Managed Workflows for Apache Airflow",
		"neptune":                  "Neptune",
		"networkmonitor":           "CloudWatch Network Monitor",
		"omics":                    "HealthOmics",
		"opensearch":               "OpenSearch",
		"opsworks":                 "OpsWorks",
		"organizations":            "Organizations",
		"personalize":              "Personalize",
		"pinpoint":                 "Pinpoint",
		"pipes":                    "EventBridge Pipes",
		"polly":                    "Polly",
		"qldb":                     "QLDB",
		"qldbsession":              "QLDB Session",
		"quicksight":               "QuickSight",
		"ram":                      "Resource Access Manager",
		"rds":                      "RDS",
		"rdsdata":                  "RDS Data",
		"redshift":                 "Redshift",
		"redshiftdata":             "Redshift Data",
		"rekognition":              "Rekognition",
		"resourcegroups":           "Resource Groups",
		"resourcegroupstaggingapi": "Resource Groups Tagging API",
	}
}

func displayNamesD() map[string]string {
	return map[string]string{
		"rolesanywhere":       "IAM Roles Anywhere",
		"route53":             "Route 53",
		"route53resolver":     "Route 53 Resolver",
		"s3":                  "S3",
		"s3control":           "S3 Control",
		"s3tables":            "S3 Tables",
		"sagemaker":           "SageMaker",
		"sagemakerruntime":    "SageMaker Runtime",
		"scheduler":           "EventBridge Scheduler",
		"secretsmanager":      "Secrets Manager",
		"securityhub":         "Security Hub",
		"serverlessrepo":      "Serverless Application Repository",
		"servicediscovery":    "Cloud Map",
		"ses":                 "SES",
		"sesv2":               "SES v2",
		"shield":              "Shield",
		"sns":                 "SNS",
		"sqs":                 "SQS",
		"ssm":                 "Systems Manager",
		"ssoadmin":            "IAM Identity Center (SSO)",
		"stepfunctions":       "Step Functions",
		"sts":                 "STS",
		"support":             "Support",
		"swf":                 "SWF",
		"textract":            "Textract",
		"timestreamquery":     "Timestream Query",
		"timestreamwrite":     "Timestream Write",
		"transcribe":          "Transcribe",
		"transfer":            "Transfer Family",
		"translate":           "Translate",
		"verifiedpermissions": "Verified Permissions",
		"vpclattice":          "VPC Lattice",
		"waf":                 "WAF",
		"wafv2":               "WAFv2",
		"workmail":            "WorkMail",
		"workspaces":          "WorkSpaces",
		"xray":                "X-Ray",
	}
}

// displayName returns the curated friendly name for slug, falling back to a
// title-cased rendering of the slug for anything unmapped.
func displayName(slug string) string {
	if name, ok := displayNameOverrides()[slug]; ok {
		return name
	}

	return titleCaseSlug(slug)
}

// titleCaseSlug capitalizes the first letter of an otherwise-unmapped slug.
// Gopherstack slugs are single run-together words with no separators, so
// this is a best-effort fallback, not a real word-boundary title case.
func titleCaseSlug(slug string) string {
	if slug == "" {
		return slug
	}

	return strings.ToUpper(slug[:1]) + slug[1:]
}
