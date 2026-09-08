package terraform_test

import (
	"archive/zip"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"text/template"

	"github.com/aws/aws-sdk-go-v2/aws"
	acmsvc "github.com/aws/aws-sdk-go-v2/service/acm"
	acmpcasvc "github.com/aws/aws-sdk-go-v2/service/acmpca"
	amplifysdkv2 "github.com/aws/aws-sdk-go-v2/service/amplify"
	apigwsvc "github.com/aws/aws-sdk-go-v2/service/apigateway"
	apigwv2svc "github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	appconfigsvc "github.com/aws/aws-sdk-go-v2/service/appconfig"
	appconfigtypes "github.com/aws/aws-sdk-go-v2/service/appconfig/types"
	appconfigdatasvc "github.com/aws/aws-sdk-go-v2/service/appconfigdata"
	applicationautoscalingsvc "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling"
	applicationautoscalingtypes "github.com/aws/aws-sdk-go-v2/service/applicationautoscaling/types"
	appsyncsdkv2 "github.com/aws/aws-sdk-go-v2/service/appsync"
	appsyncsdktypes "github.com/aws/aws-sdk-go-v2/service/appsync/types"
	athenasdkv2 "github.com/aws/aws-sdk-go-v2/service/athena"
	autoscalingsvc "github.com/aws/aws-sdk-go-v2/service/autoscaling"
	backupsvc "github.com/aws/aws-sdk-go-v2/service/backup"
	batchsvc "github.com/aws/aws-sdk-go-v2/service/batch"
	bedrocksvc "github.com/aws/aws-sdk-go-v2/service/bedrock"
	bedrockagentsvc "github.com/aws/aws-sdk-go-v2/service/bedrockagent"
	bedrockruntimesvc "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	bedrockruntimetypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	cloudcontrolsvc "github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	cfnsvc "github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cloudfrontsvc "github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cloudtrailsvc "github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	cwsvc "github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	cwlogssvc "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	codeartifactsvc "github.com/aws/aws-sdk-go-v2/service/codeartifact"
	codebuildsvc "github.com/aws/aws-sdk-go-v2/service/codebuild"
	codecommitsvc "github.com/aws/aws-sdk-go-v2/service/codecommit"
	codeconnectionssvc "github.com/aws/aws-sdk-go-v2/service/codeconnections"
	codedeploysvc "github.com/aws/aws-sdk-go-v2/service/codedeploy"
	codepipelinesvc "github.com/aws/aws-sdk-go-v2/service/codepipeline"
	codestarconnectionssvc "github.com/aws/aws-sdk-go-v2/service/codestarconnections"
	cognitoidentitysvc "github.com/aws/aws-sdk-go-v2/service/cognitoidentity"
	cognitoidpsvc "github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	configsvc "github.com/aws/aws-sdk-go-v2/service/configservice"
	cesvc "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	dmssvc "github.com/aws/aws-sdk-go-v2/service/databasemigrationservice"
	dmstypes "github.com/aws/aws-sdk-go-v2/service/databasemigrationservice/types"
	docdbsvc "github.com/aws/aws-sdk-go-v2/service/docdb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	dynamodbstreamssvc "github.com/aws/aws-sdk-go-v2/service/dynamodbstreams"
	ec2svc "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	ecrsvc "github.com/aws/aws-sdk-go-v2/service/ecr"
	ecssvc "github.com/aws/aws-sdk-go-v2/service/ecs"
	efssvc "github.com/aws/aws-sdk-go-v2/service/efs"
	ekssvc "github.com/aws/aws-sdk-go-v2/service/eks"
	elasticachesvc "github.com/aws/aws-sdk-go-v2/service/elasticache"
	elasticbeanstalksvc "github.com/aws/aws-sdk-go-v2/service/elasticbeanstalk"
	elbsvc "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancing"
	elbv2svc "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elasticsearchsvc "github.com/aws/aws-sdk-go-v2/service/elasticsearchservice"
	emrsvc "github.com/aws/aws-sdk-go-v2/service/emr"
	emrserverlesssvc "github.com/aws/aws-sdk-go-v2/service/emrserverless"
	ebsvc "github.com/aws/aws-sdk-go-v2/service/eventbridge"
	firehosesvc "github.com/aws/aws-sdk-go-v2/service/firehose"
	fissvc "github.com/aws/aws-sdk-go-v2/service/fis"
	fistypes "github.com/aws/aws-sdk-go-v2/service/fis/types"
	glaciersvc "github.com/aws/aws-sdk-go-v2/service/glacier"
	gluesvc "github.com/aws/aws-sdk-go-v2/service/glue"
	iamsvc "github.com/aws/aws-sdk-go-v2/service/iam"
	identitystoresvc "github.com/aws/aws-sdk-go-v2/service/identitystore"
	identitystoretypes "github.com/aws/aws-sdk-go-v2/service/identitystore/types"
	iotsvc "github.com/aws/aws-sdk-go-v2/service/iot"
	kafkasvc "github.com/aws/aws-sdk-go-v2/service/kafka"
	kinesissvc "github.com/aws/aws-sdk-go-v2/service/kinesis"
	kinesisanalyticssvc "github.com/aws/aws-sdk-go-v2/service/kinesisanalytics"
	kinesisanalyticsv2svc "github.com/aws/aws-sdk-go-v2/service/kinesisanalyticsv2"
	kmssvc "github.com/aws/aws-sdk-go-v2/service/kms"
	lakeformationsvc "github.com/aws/aws-sdk-go-v2/service/lakeformation"
	lambdasvc "github.com/aws/aws-sdk-go-v2/service/lambda"
	mediaconvertsvc "github.com/aws/aws-sdk-go-v2/service/mediaconvert"
	mediastoresvc "github.com/aws/aws-sdk-go-v2/service/mediastore"
	memorydbsvc "github.com/aws/aws-sdk-go-v2/service/memorydb"
	mqsvc "github.com/aws/aws-sdk-go-v2/service/mq"
	mwaasvc "github.com/aws/aws-sdk-go-v2/service/mwaa"
	neptunesvc "github.com/aws/aws-sdk-go-v2/service/neptune"
	opensearchsvc "github.com/aws/aws-sdk-go-v2/service/opensearch"
	organizationssvc "github.com/aws/aws-sdk-go-v2/service/organizations"
	pinpointsvc "github.com/aws/aws-sdk-go-v2/service/pinpoint"
	pipessvc "github.com/aws/aws-sdk-go-v2/service/pipes"
	ramsvc "github.com/aws/aws-sdk-go-v2/service/ram"
	ramsvc_types "github.com/aws/aws-sdk-go-v2/service/ram/types"
	rdssvc "github.com/aws/aws-sdk-go-v2/service/rds"
	rdsdatasvc "github.com/aws/aws-sdk-go-v2/service/rdsdata"
	rdsdatatypes "github.com/aws/aws-sdk-go-v2/service/rdsdata/types"
	redshiftsvc "github.com/aws/aws-sdk-go-v2/service/redshift"
	redshiftdatasvc "github.com/aws/aws-sdk-go-v2/service/redshiftdata"
	resourcegroupssvc "github.com/aws/aws-sdk-go-v2/service/resourcegroups"
	taggingsvc "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	route53svc "github.com/aws/aws-sdk-go-v2/service/route53"
	route53resolversvc "github.com/aws/aws-sdk-go-v2/service/route53resolver"
	s3svc "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	s3controlsvc "github.com/aws/aws-sdk-go-v2/service/s3control"
	s3tablessvc "github.com/aws/aws-sdk-go-v2/service/s3tables"
	sagemakersvc "github.com/aws/aws-sdk-go-v2/service/sagemaker"
	sagemakerruntimesvc "github.com/aws/aws-sdk-go-v2/service/sagemakerruntime"
	schedulersvc "github.com/aws/aws-sdk-go-v2/service/scheduler"
	secretssvc "github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	servicediscoverysvc "github.com/aws/aws-sdk-go-v2/service/servicediscovery"
	sessvc "github.com/aws/aws-sdk-go-v2/service/ses"
	sfnsvc "github.com/aws/aws-sdk-go-v2/service/sfn"
	snssvc "github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	sqssvc "github.com/aws/aws-sdk-go-v2/service/sqs"
	ssmsvc "github.com/aws/aws-sdk-go-v2/service/ssm"
	ssoadminsvc "github.com/aws/aws-sdk-go-v2/service/ssoadmin"
	stssvc "github.com/aws/aws-sdk-go-v2/service/sts"
	supportsvc "github.com/aws/aws-sdk-go-v2/service/support"
	swfsvc "github.com/aws/aws-sdk-go-v2/service/swf"
	swftypes "github.com/aws/aws-sdk-go-v2/service/swf/types"
	timestreamquerysvc "github.com/aws/aws-sdk-go-v2/service/timestreamquery"
	timestreamquerytypes "github.com/aws/aws-sdk-go-v2/service/timestreamquery/types"
	transfersvc "github.com/aws/aws-sdk-go-v2/service/transfer"
	verifiedpermissionssvc "github.com/aws/aws-sdk-go-v2/service/verifiedpermissions"
	vpclatticesvc "github.com/aws/aws-sdk-go-v2/service/vpclattice"
	xraysvc "github.com/aws/aws-sdk-go-v2/service/xray"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureFS holds the embedded fixtures directory tree.
//
//go:embed fixtures
var fixtureFS embed.FS

// tofuProviderCacheDir is a shared provider cache so all tests only download
// the hashicorp/aws provider once per test run.
//
//nolint:gochecknoglobals // shared provider cache path, read-only after init
var tofuProviderCacheDir = filepath.Join(os.TempDir(), "gopherstack-tofu-provider-cache")

// preInitDirMain and preInitDirRDS are directories initialised by warmProviderCache
// in TestMain. applyTofu hard-links .terraform/ from these into each test's temp dir
// instead of running tofu init, giving each test its own independent .terraform/
// subtree (no file-lock contention on terraform.tfstate) while sharing the large
// provider binary via hard links.
//
//nolint:gochecknoglobals // set once in TestMain, read-only during parallel tests
var (
	preInitDirMain    string
	preInitDirRDS     string
	preInitDirDocDB   string
	preInitDirNeptune string
)

// tofuBinaryOnce ensures tofu is only downloaded once per test run.
//
//nolint:gochecknoglobals // lazy-init singleton for the tofu binary path
var (
	tofuBinaryOnce sync.Once
	tofuBinaryPath string
	errTofuBinary  error
)

// Sentinel errors for the OpenTofu binary download helper.
var (
	errNoTofuVersions = errors.New("no stable versions found in OpenTofu API response")
	errTofuNotInZip   = errors.New("tofu binary not found in zip archive")
)

// setupEndpoint is a shared helper for tfTestCase setup that only needs the Endpoint.
func setupEndpoint(t *testing.T, _ string) map[string]any {
	t.Helper()

	return map[string]any{
		"Endpoint": endpoint,
	}
}

// providerBlock returns the OpenTofu required_providers + provider "aws" block
// pointing all service endpoints at addr (e.g. "http://localhost:32768").
func providerBlock(addr string) string {
	return fmt.Sprintf(`terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
  required_version = ">= 1.0"
}

provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true
  s3_use_path_style           = true

  # Endpoints are listed alphabetically — keep them sorted when adding new ones.
  endpoints {
    acm             = %[1]q
    acmpca          = %[1]q
    amplify         = %[1]q
    apigateway      = %[1]q
    apigatewayv2    = %[1]q
    appconfig       = %[1]q
    applicationautoscaling = %[1]q
    athena          = %[1]q
    appsync         = %[1]q
    autoscaling     = %[1]q
    backup          = %[1]q
    batch           = %[1]q
    bedrock         = %[1]q
    bedrockagent    = %[1]q
    ce              = %[1]q
    cloudcontrol    = %[1]q
    cloudformation  = %[1]q
    cloudfront      = %[1]q
    cloudtrail      = %[1]q
    cloudwatch      = %[1]q
    cloudwatchlogs  = %[1]q
    codeartifact    = %[1]q
    codebuild       = %[1]q
    codecommit          = %[1]q
    codepipeline        = %[1]q
    codeconnections     = %[1]q
    codedeploy          = %[1]q
    codestarconnections = %[1]q
    cognitoidentity          = %[1]q
    cognitoidentityprovider  = %[1]q
    configservice   = %[1]q
    dms             = %[1]q
    dynamodb        = %[1]q
    ec2             = %[1]q
    ecr             = %[1]q
    ecs             = %[1]q
    efs             = %[1]q
    eks             = %[1]q
    elb             = %[1]q
    elbv2           = %[1]q
    elasticache     = %[1]q
    elasticbeanstalk = %[1]q
    elasticsearch   = %[1]q
    emrserverless   = %[1]q
    emr             = %[1]q
    events          = %[1]q
    firehose        = %[1]q
    fis             = %[1]q
    glacier         = %[1]q
    glue            = %[1]q
    iam             = %[1]q
    identitystore   = %[1]q
    iot             = %[1]q
    kafka           = %[1]q
    kinesisanalyticsv2 = %[1]q
    kinesis         = %[1]q
    kinesisanalytics = %[1]q
    kms             = %[1]q
    lakeformation   = %[1]q
    lambda          = %[1]q
    mediaconvert    = %[1]q
    mediastore      = %[1]q
    memorydb        = %[1]q
    mq              = %[1]q
    mwaa            = %[1]q
    opensearch      = %[1]q
    organizations   = %[1]q
    pinpoint        = %[1]q
    pipes           = %[1]q
    ram             = %[1]q
    redshift        = %[1]q
    redshiftdata    = %[1]q
    resourcegroups  = %[1]q
    resourcegroupstaggingapi = %[1]q
    route53         = %[1]q
    route53resolver = %[1]q
    s3              = %[1]q
    s3control       = %[1]q
    s3tables        = %[1]q
    sagemaker       = %[1]q
    scheduler       = %[1]q
    secretsmanager  = %[1]q
    servicediscovery = %[1]q
    serverlessrepo  = %[1]q
    ses             = %[1]q
    sesv2           = %[1]q
    sfn             = %[1]q
    shield          = %[1]q
    sns             = %[1]q
    sqs             = %[1]q
    ssm             = %[1]q
    ssoadmin        = %[1]q
    sts             = %[1]q
    swf             = %[1]q
    apprunner       = %[1]q
    comprehend      = %[1]q
    datasync        = %[1]q
    detective       = %[1]q
    dlm             = %[1]q
    ds              = %[1]q
    gluedatabrew    = %[1]q
    macie2          = %[1]q
    medialive       = %[1]q
    mediapackage    = %[1]q
    polly           = %[1]q
    quicksight      = %[1]q
    rekognition     = %[1]q
    rolesanywhere   = %[1]q
    transcribe      = %[1]q
    cleanrooms      = %[1]q
    networkmonitor  = %[1]q
    vpclattice      = %[1]q
    wafv2           = %[1]q
  }
}
`, addr)
}

// macie2ProviderBlock is like providerBlock but omits skip_requesting_account_id.
// aws_macie2_account uses the caller's account ID as its Terraform resource ID;
// with skip_requesting_account_id=true that ID is "", which removes the resource
// from state immediately after creation ("root object was present, but now absent").
// Our STS emulator returns "000000000000" so the provider gets a valid account ID.
func macie2ProviderBlock(addr string) string {
	return fmt.Sprintf(`terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
  required_version = ">= 1.0"
}

provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  s3_use_path_style           = true

  # Endpoints are listed alphabetically — keep them sorted when adding new ones.
  endpoints {
    acm             = %[1]q
    acmpca          = %[1]q
    amplify         = %[1]q
    apigateway      = %[1]q
    apigatewayv2    = %[1]q
    appconfig       = %[1]q
    applicationautoscaling = %[1]q
    athena          = %[1]q
    appsync         = %[1]q
    autoscaling     = %[1]q
    backup          = %[1]q
    batch           = %[1]q
    bedrock         = %[1]q
    bedrockagent    = %[1]q
    ce              = %[1]q
    cloudcontrol    = %[1]q
    cloudformation  = %[1]q
    cloudfront      = %[1]q
    cloudtrail      = %[1]q
    cloudwatch      = %[1]q
    cloudwatchlogs  = %[1]q
    codeartifact    = %[1]q
    codebuild       = %[1]q
    codecommit          = %[1]q
    codepipeline        = %[1]q
    codeconnections     = %[1]q
    codedeploy          = %[1]q
    codestarconnections = %[1]q
    cognitoidentity          = %[1]q
    cognitoidentityprovider  = %[1]q
    configservice   = %[1]q
    dms             = %[1]q
    dynamodb        = %[1]q
    ec2             = %[1]q
    ecr             = %[1]q
    ecs             = %[1]q
    efs             = %[1]q
    eks             = %[1]q
    elb             = %[1]q
    elbv2           = %[1]q
    elasticache     = %[1]q
    elasticbeanstalk = %[1]q
    elasticsearch   = %[1]q
    emrserverless   = %[1]q
    emr             = %[1]q
    events          = %[1]q
    firehose        = %[1]q
    fis             = %[1]q
    glacier         = %[1]q
    glue            = %[1]q
    iam             = %[1]q
    identitystore   = %[1]q
    iot             = %[1]q
    kafka           = %[1]q
    kinesisanalyticsv2 = %[1]q
    kinesis         = %[1]q
    kinesisanalytics = %[1]q
    kms             = %[1]q
    lakeformation   = %[1]q
    lambda          = %[1]q
    mediaconvert    = %[1]q
    mediastore      = %[1]q
    memorydb        = %[1]q
    mq              = %[1]q
    mwaa            = %[1]q
    opensearch      = %[1]q
    organizations   = %[1]q
    pinpoint        = %[1]q
    pipes           = %[1]q
    ram             = %[1]q
    redshift        = %[1]q
    redshiftdata    = %[1]q
    resourcegroups  = %[1]q
    resourcegroupstaggingapi = %[1]q
    route53         = %[1]q
    route53resolver = %[1]q
    s3              = %[1]q
    s3control       = %[1]q
    s3tables        = %[1]q
    sagemaker       = %[1]q
    scheduler       = %[1]q
    secretsmanager  = %[1]q
    servicediscovery = %[1]q
    serverlessrepo  = %[1]q
    ses             = %[1]q
    sesv2           = %[1]q
    sfn             = %[1]q
    shield          = %[1]q
    sns             = %[1]q
    sqs             = %[1]q
    ssm             = %[1]q
    ssoadmin        = %[1]q
    sts             = %[1]q
    swf             = %[1]q
    apprunner       = %[1]q
    comprehend      = %[1]q
    datasync        = %[1]q
    detective       = %[1]q
    dlm             = %[1]q
    ds              = %[1]q
    gluedatabrew    = %[1]q
    macie2          = %[1]q
    medialive       = %[1]q
    mediapackage    = %[1]q
    polly           = %[1]q
    quicksight      = %[1]q
    rekognition     = %[1]q
    rolesanywhere   = %[1]q
    transcribe      = %[1]q
    cleanrooms      = %[1]q
    networkmonitor  = %[1]q
    vpclattice      = %[1]q
    wafv2           = %[1]q
  }
}
`, addr)
}

// rdsProviderBlock returns an OpenTofu provider block that includes the rds endpoint.
func rdsProviderBlock(addr string) string {
	return fmt.Sprintf(`terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
  required_version = ">= 1.0"
}

provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true

  endpoints {
    rds = %[1]q
    sts = %[1]q
  }
}
`, addr)
}

// rdsdataProviderBlock returns an OpenTofu provider block that includes the rds and rdsdata endpoints.
func rdsdataProviderBlock(addr string) string {
	return fmt.Sprintf(`terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
  required_version = ">= 1.0"
}

provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true

  endpoints {
    rds = %[1]q
    sts = %[1]q
  }
}
`, addr)
}

// docdbProviderBlock returns an OpenTofu provider block that includes the docdb endpoint.
func docdbProviderBlock(addr string) string {
	return fmt.Sprintf(`terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
  required_version = ">= 1.0"
}

provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true

  endpoints {
    docdb = %[1]q
    sts   = %[1]q
  }
}
`, addr)
}

// neptuneProviderBlock returns an OpenTofu provider block that includes the neptune endpoint.
func neptuneProviderBlock(addr string) string {
	return fmt.Sprintf(`terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
  required_version = ">= 1.0"
}

provider "aws" {
  region                      = "us-east-1"
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true

  endpoints {
    neptune = %[1]q
    sts     = %[1]q
  }
}
`, addr)
}

// ensureTofuBinary returns the path to the tofu binary, downloading it if
// necessary. The download happens at most once per test run.
func ensureTofuBinary(t *testing.T) string {
	t.Helper()

	tofuBinaryOnce.Do(func() {
		if path, err := exec.LookPath("tofu"); err == nil {
			tofuBinaryPath = path

			return
		}

		tofuBinaryPath, errTofuBinary = downloadTofuBinary(slog.Default())
	})

	if errTofuBinary != nil {
		require.NoError(t, errTofuBinary, "could not obtain tofu binary")
	}

	return tofuBinaryPath
}

// downloadTofuBinary fetches the latest stable OpenTofu release for the current
// platform, extracts the binary to [os.TempDir], and returns its path.
func downloadTofuBinary(logger *slog.Logger) (string, error) {
	ctx := context.Background()

	versionReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://get.opentofu.org/tofu/api.json", nil)
	if err != nil {
		return "", fmt.Errorf("creating OpenTofu version request: %w", err)
	}
	resp, err := http.DefaultClient.Do(versionReq)
	if err != nil {
		return "", fmt.Errorf("fetching OpenTofu version list: %w", err)
	}
	defer resp.Body.Close()

	var api struct {
		Versions []struct {
			ID string `json:"id"`
		} `json:"versions"`
	}
	err = json.NewDecoder(resp.Body).Decode(&api)
	if err != nil {
		return "", fmt.Errorf("decoding OpenTofu version list: %w", err)
	}

	version := latestStableTofuVersion(api.Versions)
	if version == "" {
		return "", errNoTofuVersions
	}
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	downloadURL := fmt.Sprintf(
		"https://github.com/opentofu/opentofu/releases/download/v%s/tofu_%s_%s_%s.zip",
		version, version, goos, goarch,
	)
	logger.Info("downloading OpenTofu", "version", version, "url", downloadURL)

	zipReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating OpenTofu download request: %w", err)
	}
	zipResp, err := http.DefaultClient.Do(zipReq)
	if err != nil {
		return "", fmt.Errorf("downloading OpenTofu zip: %w", err)
	}
	defer zipResp.Body.Close()

	zipData, err := io.ReadAll(zipResp.Body)
	if err != nil {
		return "", fmt.Errorf("reading OpenTofu zip: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return "", fmt.Errorf("opening OpenTofu zip: %w", err)
	}

	binaryName := "tofu"
	if goos == "windows" {
		binaryName = "tofu.exe"
	}

	for _, f := range zr.File {
		if f.Name != binaryName {
			continue
		}

		return extractTofuBinary(f, binaryName)
	}

	return "", errTofuNotInZip
}

// extractTofuBinary writes a single [zip.File] to [os.TempDir] with executable
// permissions and returns its path.
func extractTofuBinary(f *zip.File, binaryName string) (string, error) {
	destPath := filepath.Join(os.TempDir(), binaryName)

	rc, err := f.Open()
	if err != nil {
		return "", fmt.Errorf("opening %s in zip: %w", binaryName, err)
	}
	defer rc.Close()

	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", fmt.Errorf("creating tofu binary: %w", err)
	}
	defer out.Close()

	if _, err = io.Copy(out, rc); err != nil {
		return "", fmt.Errorf("writing tofu binary: %w", err)
	}

	return destPath, nil
}

// latestStableTofuVersion returns the first stable (non-pre-release) version ID
// from the OpenTofu API versions list, or an empty string if none is found.
func latestStableTofuVersion(versions []struct {
	ID string `json:"id"`
},
) string {
	for _, v := range versions {
		if !strings.Contains(v.ID, "-") {
			return v.ID
		}
	}

	return ""
}

// hardLinkDir recreates the directory tree rooted at src inside dst, creating
// hard links for immutable files (provider binaries) and copies for mutable
// metadata files (*.tfstate). Hard-linking the provider binary (~200 MB) is
// near-instant and gives each test its own independent .terraform/ subtree.
// Mutable .tfstate files are copied so each test has a separate inode and
// writes (e.g. .terraform/terraform.tfstate) never contend across tests.
func hardLinkDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}

		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		// Copy mutable state files so each test has an independent inode.
		if strings.HasSuffix(path, ".tfstate") {
			return copyFile(path, target, info.Mode())
		}

		// Attempt to hard-link the file for efficiency.
		if linkErr := os.Link(path, target); linkErr == nil {
			return nil
		}

		// Fallback to copy if hard-linking fails (e.g. cross-device link).
		return copyFile(path, target, info.Mode())
	})
}

// copyFile copies the content of src to dst with the given file mode.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}

	if _, err = io.Copy(out, in); err != nil {
		out.Close()

		return err
	}

	if err = out.Sync(); err != nil {
		out.Close()

		return err
	}

	return out.Close()
}

// selectPreInitDir returns the pre-initialised directory whose .terraform/ subtree
// can be reused for the given HCL configuration, or an empty string if none matches.
func selectPreInitDir(hcl string) string {
	switch {
	case strings.HasPrefix(hcl, docdbProviderBlock(endpoint)):
		return preInitDirDocDB
	case strings.HasPrefix(hcl, neptuneProviderBlock(endpoint)):
		return preInitDirNeptune
	case strings.HasPrefix(hcl, rdsProviderBlock(endpoint)):
		return preInitDirRDS
	case strings.HasPrefix(hcl, providerBlock(endpoint)):
		return preInitDirMain
	default:
		return ""
	}
}

// reuseOrInit hard-links the pre-initialised .terraform/ subtree for the HCL
// configuration into dir when one is available, falling back to tofu init.
// Extracted from applyTofu to avoid deeply nested conditionals.
func reuseOrInit(t *testing.T, dir, hcl string, run func(bool, ...string) bool) {
	t.Helper()

	initSrc := selectPreInitDir(hcl)
	if initSrc == "" {
		run(true, "init", "-no-color")

		return
	}

	dotTerraform := filepath.Join(dir, ".terraform")
	if err := hardLinkDir(filepath.Join(initSrc, ".terraform"), dotTerraform); err != nil {
		t.Logf("could not hard-link .terraform from pre-init dir: %v; cleaning up and falling back to init", err)
		// Remove any partially-created tree so tofu init starts from a clean slate.
		if rmErr := os.RemoveAll(dotTerraform); rmErr != nil {
			t.Logf("failed to remove partial .terraform dir: %v", rmErr)
		}

		run(true, "init", "-no-color")

		return
	}

	// Copy the lock file (small, ~1 KB) so tofu can verify provider checksums.
	// Treat a missing or unreadable lock file as a hard error: without it tofu
	// may re-resolve providers in a non-deterministic way.
	lockData, readErr := os.ReadFile(filepath.Join(initSrc, ".terraform.lock.hcl"))
	if readErr != nil {
		t.Logf("failed to read .terraform.lock.hcl from pre-init dir: %v; falling back to init", readErr)
		if rmErr := os.RemoveAll(dotTerraform); rmErr != nil {
			t.Logf("failed to remove .terraform dir before fallback: %v", rmErr)
		}

		run(true, "init", "-no-color")

		return
	}

	if writeErr := os.WriteFile(filepath.Join(dir, ".terraform.lock.hcl"), lockData, 0o644); writeErr != nil {
		t.Logf("failed to write .terraform.lock.hcl: %v; falling back to init", writeErr)
		if rmErr := os.RemoveAll(dotTerraform); rmErr != nil {
			t.Logf("failed to remove .terraform dir before fallback: %v", rmErr)
		}

		run(true, "init", "-no-color")
	}
}

// applyTofu writes hcl to a main.tf in dir, runs tofu init (or reuses a
// pre-initialised .terraform/ via hardLinkDir), runs tofu apply, and registers
// a cleanup that destroys all created resources.
func applyTofu(t *testing.T, tofuBin, dir, hcl string) {
	t.Helper()

	cfgPath := dir + "/main.tf"
	require.NoError(t, os.WriteFile(cfgPath, []byte(hcl), 0o644))

	if err := os.MkdirAll(tofuProviderCacheDir, 0o755); err != nil {
		t.Logf("could not create provider cache dir: %v", err)
	}

	env := append(
		os.Environ(),
		"TF_IN_AUTOMATION=1",
		"TF_PLUGIN_CACHE_DIR="+tofuProviderCacheDir,
		"TF_PLUGIN_CACHE_MAY_BREAK_DEPENDENCY_LOCK_FILE=true",
		// Catch-all for services not covered by the provider endpoints block.
		// The AWS provider v5.6.0+ reads this env var as a global endpoint
		// override so services like forecast, mediatailor, personalize,
		// translate, mediastoredata, and workmail route to gopherstack.
		"AWS_ENDPOINT_URL="+endpoint,
	)

	run := func(failFatal bool, args ...string) bool {
		t.Helper()

		cmd := exec.Command(tofuBin, args...)
		cmd.Dir = dir
		cmd.Env = env

		out, err := cmd.CombinedOutput()
		t.Logf("tofu %v:\n%s", args, out)

		if err != nil {
			if failFatal {
				require.NoError(t, err, "tofu %v failed", args)
			}

			t.Logf("tofu %v failed (non-fatal): %v", args, err)

			return false
		}

		return true
	}

	// Reuse a pre-initialised .terraform directory when available.
	// hardLinkDir copies the directory tree with hard links so the large
	// provider binary is shared (no extra disk use) while each test keeps
	// its own independent directory entries — eliminating the file-lock
	// contention on .terraform/terraform.tfstate that serialised parallel runs.
	reuseOrInit(t, dir, hcl, run)

	run(true, "apply", "-auto-approve", "-no-color")

	t.Cleanup(func() {
		run(false, "destroy", "-auto-approve", "-no-color")
	})
}

// renderFixture reads fixtures/<name>.tf, renders it as a Go text/template
// with vars, and returns the resulting HCL string. name uses forward-slash
// paths, e.g. "dynamodb/success".
func renderFixture(t *testing.T, name string, vars map[string]any) string {
	t.Helper()

	path := "fixtures/" + name + ".tf"

	content, err := fixtureFS.ReadFile(path)
	require.NoError(t, err, "reading fixture %s", path)

	tmpl, err := template.New(name).Option("missingkey=error").Parse(string(content))
	require.NoError(t, err, "parsing fixture %s", path)

	var buf strings.Builder
	require.NoError(t, tmpl.Execute(&buf, vars), "rendering fixture %s", path)

	return buf.String()
}

// vpcCIDRVars returns VPCCidr/SubnetCidrA/SubnetCidrB template vars for a
// fixture that provisions its own VPC, derived from a stable hash of the
// test's name. This keeps CIDRs identical across runs while guaranteeing
// distinct parallel subtests don't allocate the same 10.x.0.0/16 block —
// EC2's CreateVpc rejects a CIDR overlapping any existing VPC account-wide,
// and the shared test container puts every parallel subtest in that account.
func vpcCIDRVars(t *testing.T) map[string]any {
	t.Helper()

	h := fnv.New32a()
	_, _ = h.Write([]byte(t.Name()))
	octet := int(h.Sum32()%254) + 1

	return map[string]any{
		"VPCCidr":     fmt.Sprintf("10.%d.0.0/16", octet),
		"SubnetCidrA": fmt.Sprintf("10.%d.1.0/24", octet),
		"SubnetCidrB": fmt.Sprintf("10.%d.2.0/24", octet),
	}
}

// tfTestCase describes one scenario within a per-service Terraform test table.
// Add new entries to a service's []tfTestCase slice to cover additional inputs
// or failure paths without touching the test runner.
type tfTestCase struct {
	providerFn func(addr string) string
	setup      func(t *testing.T, dir string) map[string]any
	verify     func(t *testing.T, ctx context.Context, vars map[string]any)
	name       string
	fixture    string
}

// runTFTest is the common runner shared by all per-service Terraform tests.
// It renders the fixture, applies Terraform, then calls verify (if set).
func runTFTest(t *testing.T, tc tfTestCase) {
	t.Helper()
	dumpContainerLogsOnFailure(t)

	tofuBin := ensureTofuBinary(t)
	ctx := t.Context()
	dir := t.TempDir()

	provFn := tc.providerFn
	if provFn == nil {
		provFn = providerBlock
	}

	vars := tc.setup(t, dir)
	hcl := provFn(endpoint) + renderFixture(t, tc.fixture, vars)
	applyTofu(t, tofuBin, dir, hcl)

	if tc.verify != nil {
		tc.verify(t, ctx, vars)
	}
}

// runTfTestWithEndpoint is a specialized runner for tests using setupEndpoint.
func runTfTestWithEndpoint(t *testing.T, fixture string, verify func(t *testing.T, ctx context.Context)) {
	t.Helper()

	runTFTest(t, tfTestCase{
		name:    "success",
		fixture: fixture,
		setup:   setupEndpoint,
		verify: func(t *testing.T, ctx context.Context, _ map[string]any) {
			t.Helper()
			verify(t, ctx)
		},
	})
}

// TestTerraform_DynamoDB provisions a DynamoDB table and verifies key schema.
func TestTerraform_DynamoDB(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "dynamodb/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"TableName": "tf-ddb-" + uuid.NewString()}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createDynamoDBClient(t)
				out, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
					TableName: aws.String(vars["TableName"].(string)),
				})
				require.NoError(t, err, "DescribeTable should succeed after terraform apply")
				require.NotNil(t, out.Table)
				assert.Equal(t, vars["TableName"].(string), aws.ToString(out.Table.TableName))
				require.Len(t, out.Table.KeySchema, 1)
				assert.Equal(t, "pk", aws.ToString(out.Table.KeySchema[0].AttributeName))
				assert.Equal(t, ddbtypes.KeyTypeHash, out.Table.KeySchema[0].KeyType)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_DynamoDBStreams provisions a DynamoDB table with streams enabled via Terraform
// and verifies the stream is visible via the DynamoDB Streams SDK.
func TestTerraform_DynamoDBStreams(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "streams_enabled",
			fixture: "dynamodbstreams/streams_enabled",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"TableName": "tf-ddbstreams-" + uuid.NewString()[:8]}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createDynamoDBStreamsClient(t)
				tableName := vars["TableName"].(string)

				out, err := client.ListStreams(ctx, &dynamodbstreamssvc.ListStreamsInput{
					TableName: aws.String(tableName),
				})
				require.NoError(t, err, "ListStreams should succeed after terraform apply")
				require.NotEmpty(t, out.Streams, "stream should be listed for table %q", tableName)
				assert.Equal(t, tableName, aws.ToString(out.Streams[0].TableName))
				assert.NotEmpty(t, aws.ToString(out.Streams[0].StreamArn), "stream ARN should be non-empty")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_DynamoDBComprehensive provisions tables with streams, global tables, TTL, and
// on-demand billing via Terraform and verifies each feature via the DynamoDB SDK.
func TestTerraform_DynamoDBComprehensive(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "comprehensive",
			fixture: "dynamodb-comprehensive/main",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				suffix := uuid.NewString()[:8]

				return map[string]any{
					"StreamsTable":  "tf-ddb-streams-" + suffix,
					"GlobalTable":   "tf-ddb-global-" + suffix,
					"OnDemandTable": "tf-ddb-ondemand-" + suffix,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createDynamoDBClient(t)
				streamsClient := createDynamoDBStreamsClient(t)

				streamsTable := vars["StreamsTable"].(string)
				globalTable := vars["GlobalTable"].(string)
				onDemandTable := vars["OnDemandTable"].(string)

				// --- Streams table: stream ARN present, TTL enabled ---
				descOut, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
					TableName: aws.String(streamsTable),
				})
				require.NoError(t, err, "DescribeTable(streams) should succeed")
				require.NotNil(t, descOut.Table)
				assert.NotEmpty(t, aws.ToString(descOut.Table.LatestStreamArn), "stream ARN should be set")
				require.NotNil(t, descOut.Table.StreamSpecification)
				assert.True(t, aws.ToBool(descOut.Table.StreamSpecification.StreamEnabled))
				assert.Equal(
					t,
					ddbtypes.StreamViewTypeNewAndOldImages,
					descOut.Table.StreamSpecification.StreamViewType,
				)

				// TTL should be enabled on the streams table.
				ttlOut, err := client.DescribeTimeToLive(ctx, &dynamodb.DescribeTimeToLiveInput{
					TableName: aws.String(streamsTable),
				})
				require.NoError(t, err, "DescribeTimeToLive should succeed")
				require.NotNil(t, ttlOut.TimeToLiveDescription)
				assert.Equal(t, ddbtypes.TimeToLiveStatusEnabled, ttlOut.TimeToLiveDescription.TimeToLiveStatus)
				assert.Equal(t, "expires_at", aws.ToString(ttlOut.TimeToLiveDescription.AttributeName))

				// Stream should be listed.
				listStreamsOut, err := streamsClient.ListStreams(ctx, &dynamodbstreamssvc.ListStreamsInput{
					TableName: aws.String(streamsTable),
				})
				require.NoError(t, err, "ListStreams should succeed")
				require.NotEmpty(t, listStreamsOut.Streams, "at least one stream should be listed")
				assert.Equal(t, streamsTable, aws.ToString(listStreamsOut.Streams[0].TableName))

				// --- Global table: GlobalTableVersion present ---
				descGlobal, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
					TableName: aws.String(globalTable),
				})
				require.NoError(t, err, "DescribeTable(global) should succeed")
				require.NotNil(t, descGlobal.Table)
				assert.NotEmpty(t, aws.ToString(descGlobal.Table.GlobalTableVersion),
					"GlobalTableVersion should be set for a global table")

				// DescribeGlobalTable should return the table.
				descGT, err := client.DescribeGlobalTable(ctx, &dynamodb.DescribeGlobalTableInput{
					GlobalTableName: aws.String(globalTable),
				})
				require.NoError(t, err, "DescribeGlobalTable should succeed")
				require.NotNil(t, descGT.GlobalTableDescription)
				assert.Equal(t, globalTable, aws.ToString(descGT.GlobalTableDescription.GlobalTableName))

				// --- On-demand billing table ---
				descOnDemand, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
					TableName: aws.String(onDemandTable),
				})
				require.NoError(t, err, "DescribeTable(on-demand) should succeed")
				require.NotNil(t, descOnDemand.Table)
				require.NotNil(t, descOnDemand.Table.BillingModeSummary)
				assert.Equal(t, ddbtypes.BillingModePayPerRequest,
					descOnDemand.Table.BillingModeSummary.BillingMode,
					"on-demand table should report PAY_PER_REQUEST billing mode")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_S3AndSQS provisions an S3 bucket and SQS queue and verifies both.
func TestTerraform_S3AndSQS(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "s3_sqs/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()

				return map[string]any{
					"BucketName": "tf-s3-" + id,
					"QueueName":  "tf-sqs-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				s3Client := createS3Client(t)
				_, err := s3Client.HeadBucket(ctx, &s3svc.HeadBucketInput{
					Bucket: aws.String(vars["BucketName"].(string)),
				})
				require.NoError(t, err, "HeadBucket should succeed after terraform apply")

				sqsClient := createSQSClient(t)
				getURLOut, err := sqsClient.GetQueueUrl(ctx, &sqssvc.GetQueueUrlInput{
					QueueName: aws.String(vars["QueueName"].(string)),
				})
				require.NoError(t, err, "GetQueueUrl should succeed after terraform apply")
				assert.Contains(t, aws.ToString(getURLOut.QueueUrl), vars["QueueName"].(string))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_RDS provisions an RDS DB instance and verifies it exists.
func TestTerraform_RDS(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:       "success",
			fixture:    "rds/success",
			providerFn: rdsProviderBlock,
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"Identifier": "tf-rds-" + uuid.NewString()[:8]}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createRDSClient(t)
				out, err := client.DescribeDBInstances(ctx, &rdssvc.DescribeDBInstancesInput{
					DBInstanceIdentifier: aws.String(vars["Identifier"].(string)),
				})
				require.NoError(t, err, "DescribeDBInstances should succeed after terraform apply")
				require.Len(t, out.DBInstances, 1)
				assert.Equal(t, vars["Identifier"].(string), aws.ToString(out.DBInstances[0].DBInstanceIdentifier))
				assert.Equal(t, "postgres", aws.ToString(out.DBInstances[0].Engine))
				assert.Equal(t, "available", aws.ToString(out.DBInstances[0].DBInstanceStatus))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_DocDB provisions a DocDB cluster and instance and verifies they exist.
func TestTerraform_DocDB(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:       "success",
			fixture:    "docdb/success",
			providerFn: docdbProviderBlock,
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"Suffix": uuid.NewString()[:8]}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				suffix := vars["Suffix"].(string)
				client := createDocDBClient(t)
				clusterID := "tf-docdb-" + suffix
				out, err := client.DescribeDBClusters(ctx, &docdbsvc.DescribeDBClustersInput{
					DBClusterIdentifier: &clusterID,
				})
				require.NoError(t, err, "DescribeDBClusters should succeed after terraform apply")
				require.Len(t, out.DBClusters, 1)
				assert.Equal(t, clusterID, *out.DBClusters[0].DBClusterIdentifier)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Neptune provisions a Neptune cluster and instance and verifies they exist.
func TestTerraform_Neptune(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:       "success",
			fixture:    "neptune/success",
			providerFn: neptuneProviderBlock,
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"Suffix": uuid.NewString()[:8]}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				suffix := vars["Suffix"].(string)
				client := createNeptuneClient(t)
				clusterID := "tf-neptune-" + suffix
				out, err := client.DescribeDBClusters(ctx, &neptunesvc.DescribeDBClustersInput{
					DBClusterIdentifier: &clusterID,
				})
				require.NoError(t, err, "DescribeDBClusters should succeed after terraform apply")
				require.Len(t, out.DBClusters, 1)
				assert.Equal(t, clusterID, *out.DBClusters[0].DBClusterIdentifier)

				instanceID := "tf-neptune-inst-" + suffix
				instOut, err := client.DescribeDBInstances(ctx, &neptunesvc.DescribeDBInstancesInput{
					DBInstanceIdentifier: &instanceID,
				})
				require.NoError(t, err, "DescribeDBInstances should succeed after terraform apply")
				require.Len(t, instOut.DBInstances, 1)
				assert.Equal(t, instanceID, *instOut.DBInstances[0].DBInstanceIdentifier)

				sgName := "tf-neptune-sg-" + suffix
				sgOut, err := client.DescribeDBSubnetGroups(ctx, &neptunesvc.DescribeDBSubnetGroupsInput{
					DBSubnetGroupName: &sgName,
				})
				require.NoError(t, err, "DescribeDBSubnetGroups should succeed after terraform apply")
				require.Len(t, sgOut.DBSubnetGroups, 1)
				assert.Equal(t, sgName, *sgOut.DBSubnetGroups[0].DBSubnetGroupName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Lambda provisions a Lambda function and verifies it exists.
func TestTerraform_Lambda(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "lambda/success",
			setup: func(t *testing.T, dir string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				// Create a minimal zip file containing a Python handler.
				var buf bytes.Buffer
				zw := zip.NewWriter(&buf)
				f, err := zw.Create("index.py")
				require.NoError(t, err)
				_, err = f.Write([]byte("def handler(event, context):\n    return {}\n"))
				require.NoError(t, err)
				require.NoError(t, zw.Close())

				zipPath := filepath.Join(dir, "function.zip")
				require.NoError(t, os.WriteFile(zipPath, buf.Bytes(), 0o644))

				return map[string]any{
					"FuncName": "tf-lambda-" + id,
					"RoleName": "tf-lambda-role-" + id,
					"ZipPath":  zipPath,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createLambdaClient(t)
				out, err := client.GetFunction(ctx, &lambdasvc.GetFunctionInput{
					FunctionName: aws.String(vars["FuncName"].(string)),
				})
				require.NoError(t, err, "GetFunction should succeed after terraform apply")
				require.NotNil(t, out.Configuration)
				assert.Equal(t, vars["FuncName"].(string), aws.ToString(out.Configuration.FunctionName))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Lambda_ProvisionedConcurrency provisions a Lambda function with provisioned
// concurrency and verifies the config exists.
func TestTerraform_Lambda_ProvisionedConcurrency(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "lambda/provisioned-concurrency",
			setup: func(t *testing.T, dir string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				var buf bytes.Buffer
				zw := zip.NewWriter(&buf)
				f, err := zw.Create("index.py")
				require.NoError(t, err)
				_, err = f.Write([]byte("def handler(event, context):\n    return {}\n"))
				require.NoError(t, err)
				require.NoError(t, zw.Close())

				zipPath := filepath.Join(dir, "function.zip")
				require.NoError(t, os.WriteFile(zipPath, buf.Bytes(), 0o644))

				return map[string]any{
					"FuncName": "tf-prov-" + id,
					"RoleName": "tf-prov-role-" + id,
					"ZipPath":  zipPath,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createLambdaClient(t)

				// Publish version should have occurred, so version "1" should exist.
				listOut, err := client.ListProvisionedConcurrencyConfigs(
					ctx,
					&lambdasvc.ListProvisionedConcurrencyConfigsInput{
						FunctionName: aws.String(vars["FuncName"].(string)),
					},
				)
				require.NoError(t, err, "ListProvisionedConcurrencyConfigs should succeed after terraform apply")
				assert.NotEmpty(t, listOut.ProvisionedConcurrencyConfigs,
					"at least one provisioned concurrency config should exist")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_IAM provisions an IAM role, policy, and attachment and verifies the role.
func TestTerraform_IAM(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "iam/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"RoleName":   "tf-role-" + id,
					"PolicyName": "tf-policy-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createIAMClient(t)
				out, err := client.GetRole(ctx, &iamsvc.GetRoleInput{
					RoleName: aws.String(vars["RoleName"].(string)),
				})
				require.NoError(t, err, "GetRole should succeed after terraform apply")
				require.NotNil(t, out.Role)
				assert.Equal(t, vars["RoleName"].(string), aws.ToString(out.Role.RoleName))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_SNSSQSSubscription provisions an SNS topic, SQS queue, and
// subscription, and verifies the topic exists.
func TestTerraform_SNSSQSSubscription(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "sns_sqs/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"TopicName": "tf-topic-" + id,
					"QueueName": "tf-queue-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createSNSClient(t)
				out, err := client.GetTopicAttributes(ctx, &snssvc.GetTopicAttributesInput{
					TopicArn: aws.String("arn:aws:sns:us-east-1:000000000000:" + vars["TopicName"].(string)),
				})
				require.NoError(t, err, "GetTopicAttributes should succeed after terraform apply")
				assert.NotEmpty(t, out.Attributes)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_SNSPlatformApplication provisions an SNS platform application and verifies it exists.
func TestTerraform_SNSPlatformApplication(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "sns_platform/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{
					"AppName": "tf-app-" + uuid.NewString()[:8],
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createSNSClient(t)
				out, err := client.ListPlatformApplications(ctx, &snssvc.ListPlatformApplicationsInput{})
				require.NoError(t, err, "ListPlatformApplications should succeed after terraform apply")

				appName := vars["AppName"].(string)
				found := slices.ContainsFunc(out.PlatformApplications, func(app snstypes.PlatformApplication) bool {
					return strings.HasSuffix(aws.ToString(app.PlatformApplicationArn), "/"+appName)
				})
				assert.True(t, found, "platform application %q should exist after terraform apply", appName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_KMS provisions a KMS key and alias and verifies the key exists.
func TestTerraform_KMS(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "kms/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"KeyDesc":   "tf-test-key-" + id,
					"AliasName": "alias/tf-test-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createKMSClient(t)
				out, err := client.DescribeKey(ctx, &kmssvc.DescribeKeyInput{
					KeyId: aws.String(vars["AliasName"].(string)),
				})
				require.NoError(t, err, "DescribeKey should succeed after terraform apply")
				require.NotNil(t, out.KeyMetadata)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_SecretsManager provisions a secret and version and verifies the secret.
func TestTerraform_SecretsManager(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "secretsmanager/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"SecretName": "tf-secret-" + uuid.NewString()[:8]}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createSecretsManagerClient(t)
				out, err := client.DescribeSecret(ctx, &secretssvc.DescribeSecretInput{
					SecretId: aws.String(vars["SecretName"].(string)),
				})
				require.NoError(t, err, "DescribeSecret should succeed after terraform apply")
				assert.Equal(t, vars["SecretName"].(string), aws.ToString(out.Name))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_SSMParameter provisions an SSM parameter and verifies it exists.
func TestTerraform_SSMParameter(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "ssm/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"ParamName": "/tf/test/" + uuid.NewString()[:8]}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createSSMClient(t)
				out, err := client.GetParameter(ctx, &ssmsvc.GetParameterInput{
					Name: aws.String(vars["ParamName"].(string)),
				})
				require.NoError(t, err, "GetParameter should succeed after terraform apply")
				require.NotNil(t, out.Parameter)
				assert.Equal(t, vars["ParamName"].(string), aws.ToString(out.Parameter.Name))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Route53 provisions a hosted zone and record and verifies the zone.
func TestTerraform_Route53(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "route53/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"ZoneName": "tf-test-" + uuid.NewString()[:8] + ".example.com"}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createRoute53Client(t)
				out, err := client.ListHostedZones(ctx, &route53svc.ListHostedZonesInput{})
				require.NoError(t, err, "ListHostedZones should succeed after terraform apply")

				zoneName := vars["ZoneName"].(string)
				found := false

				for _, zone := range out.HostedZones {
					if aws.ToString(zone.Name) == zoneName+"." || aws.ToString(zone.Name) == zoneName {
						found = true

						break
					}
				}

				assert.True(t, found, "hosted zone %q should exist after terraform apply", zoneName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_CloudWatchLogGroup provisions a log group and verifies it exists.
func TestTerraform_CloudWatchLogGroup(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "cloudwatchlogs/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"LogGroupName": "/tf/test/" + uuid.NewString()[:8]}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createCloudWatchLogsClient(t)
				out, err := client.DescribeLogGroups(ctx, &cwlogssvc.DescribeLogGroupsInput{
					LogGroupNamePrefix: aws.String(vars["LogGroupName"].(string)),
				})
				require.NoError(t, err, "DescribeLogGroups should succeed after terraform apply")

				logGroupName := vars["LogGroupName"].(string)
				found := false

				for _, lg := range out.LogGroups {
					if aws.ToString(lg.LogGroupName) == logGroupName {
						found = true

						break
					}
				}

				assert.True(t, found, "log group %q should exist after terraform apply", logGroupName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_SESEmailIdentity provisions an SES email identity and verifies it is listed.
func TestTerraform_SESEmailIdentity(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "ses/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"Email": "tf-test-" + uuid.NewString()[:8] + "@example.com"}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createSESClient(t)
				out, err := client.ListIdentities(ctx, &sessvc.ListIdentitiesInput{})
				require.NoError(t, err, "ListIdentities should succeed after terraform apply")

				email := vars["Email"].(string)
				assert.True(t, slices.Contains(out.Identities, email),
					"email identity %q should be listed after terraform apply", email)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_DataSources provisions data sources and an S3 bucket and verifies the apply succeeds.
func TestTerraform_DataSources(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "datasources/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"BucketName": "tf-ds-test-" + uuid.NewString()[:8]}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_StepFunctions provisions a Step Functions state machine and verifies it exists.
func TestTerraform_StepFunctions(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "stepfunctions/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"SMName":   "tf-sfn-" + id,
					"RoleName": "tf-sfn-role-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createSFNClient(t)
				out, err := client.ListStateMachines(ctx, &sfnsvc.ListStateMachinesInput{})
				require.NoError(t, err, "ListStateMachines should succeed after terraform apply")

				smName := vars["SMName"].(string)
				found := false

				for _, sm := range out.StateMachines {
					if aws.ToString(sm.Name) == smName {
						found = true

						break
					}
				}

				assert.True(t, found, "state machine %q should exist after terraform apply", smName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_EventBridge provisions an EventBridge bus, rule, archive, connection and
// API destination via Terraform, then verifies each resource via the AWS SDK.
func TestTerraform_EventBridge(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "eventbridge/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"BusName":            "tf-bus-" + id,
					"RuleName":           "tf-rule-" + id,
					"ArchiveName":        "tf-arc-" + id,
					"ConnectionName":     "tf-conn-" + id,
					"APIDestinationName": "tf-dst-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createEventBridgeClient(t)

				// Verify rule.
				out, err := client.DescribeRule(ctx, &ebsvc.DescribeRuleInput{
					Name:         aws.String(vars["RuleName"].(string)),
					EventBusName: aws.String(vars["BusName"].(string)),
				})
				require.NoError(t, err, "DescribeRule should succeed after terraform apply")
				assert.Equal(t, vars["RuleName"].(string), aws.ToString(out.Name))

				// Verify archive.
				archOut, err := client.DescribeArchive(ctx, &ebsvc.DescribeArchiveInput{
					ArchiveName: aws.String(vars["ArchiveName"].(string)),
				})
				require.NoError(t, err, "DescribeArchive should succeed after terraform apply")
				assert.Equal(t, vars["ArchiveName"].(string), aws.ToString(archOut.ArchiveName))
				assert.Equal(t, aws.Int32(7), archOut.RetentionDays)

				// Verify connection.
				connOut, err := client.DescribeConnection(ctx, &ebsvc.DescribeConnectionInput{
					Name: aws.String(vars["ConnectionName"].(string)),
				})
				require.NoError(t, err, "DescribeConnection should succeed after terraform apply")
				assert.Equal(t, vars["ConnectionName"].(string), aws.ToString(connOut.Name))

				// Verify API destination.
				dstOut, err := client.DescribeApiDestination(ctx, &ebsvc.DescribeApiDestinationInput{
					Name: aws.String(vars["APIDestinationName"].(string)),
				})
				require.NoError(t, err, "DescribeApiDestination should succeed after terraform apply")
				assert.Equal(t, vars["APIDestinationName"].(string), aws.ToString(dstOut.Name))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_CloudWatchAlarm provisions a CloudWatch metric alarm and verifies it exists.
func TestTerraform_CloudWatchAlarm(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "cloudwatch/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"AlarmName": "tf-alarm-" + uuid.NewString()[:8]}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createCloudWatchClient(t)
				out, err := client.DescribeAlarms(ctx, &cwsvc.DescribeAlarmsInput{
					AlarmNames: []string{vars["AlarmName"].(string)},
				})
				require.NoError(t, err, "DescribeAlarms should succeed after terraform apply")
				require.Len(t, out.MetricAlarms, 1)
				assert.Equal(t, vars["AlarmName"].(string), aws.ToString(out.MetricAlarms[0].AlarmName))
				assert.Equal(t, cwtypes.ComparisonOperatorGreaterThanThreshold, out.MetricAlarms[0].ComparisonOperator)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Kinesis provisions a Kinesis stream and verifies it exists.
func TestTerraform_Kinesis(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "kinesis/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"StreamName": "tf-kinesis-" + uuid.NewString()[:8]}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createKinesisClient(t)
				out, err := client.DescribeStreamSummary(ctx, &kinesissvc.DescribeStreamSummaryInput{
					StreamName: aws.String(vars["StreamName"].(string)),
				})
				require.NoError(t, err, "DescribeStreamSummary should succeed after terraform apply")
				require.NotNil(t, out.StreamDescriptionSummary)
				assert.Equal(t, vars["StreamName"].(string), aws.ToString(out.StreamDescriptionSummary.StreamName))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_ACM provisions an ACM certificate and verifies it is listed.
func TestTerraform_ACM(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "acm/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"Domain": "tf-acm-" + uuid.NewString()[:8] + ".example.com"}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createACMClient(t)
				out, err := client.ListCertificates(ctx, &acmsvc.ListCertificatesInput{})
				require.NoError(t, err, "ListCertificates should succeed after terraform apply")

				domain := vars["Domain"].(string)
				found := false

				for _, cert := range out.CertificateSummaryList {
					if aws.ToString(cert.DomainName) == domain {
						found = true

						break
					}
				}

				assert.True(t, found, "certificate for %q should exist after terraform apply", domain)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_ACMPCA provisions an ACM PCA certificate authority and verifies it appears in list output.
func TestTerraform_ACMPCA(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "acmpca/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"CommonName": "tf-test-root-ca-" + uuid.NewString()[:8]}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createACMPCAClient(t)
				out, err := client.ListCertificateAuthorities(ctx, &acmpcasvc.ListCertificateAuthoritiesInput{})
				require.NoError(t, err, "ListCertificateAuthorities should succeed after terraform apply")

				commonName := vars["CommonName"].(string)
				found := false

				for _, ca := range out.CertificateAuthorities {
					if ca.CertificateAuthorityConfiguration != nil &&
						ca.CertificateAuthorityConfiguration.Subject != nil &&
						aws.ToString(ca.CertificateAuthorityConfiguration.Subject.CommonName) == commonName {
						found = true

						break
					}
				}

				assert.True(t, found, "CA with common name %q should exist after terraform apply", commonName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_CloudFormation provisions a CloudFormation stack and verifies it exists.
func TestTerraform_CloudFormation(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "cloudformation/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"StackName": "tf-cfn-" + uuid.NewString()[:8]}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createCloudFormationClient(t)
				out, err := client.DescribeStacks(ctx, &cfnsvc.DescribeStacksInput{
					StackName: aws.String(vars["StackName"].(string)),
				})
				require.NoError(t, err, "DescribeStacks should succeed after terraform apply")
				require.Len(t, out.Stacks, 1)
				assert.Equal(t, vars["StackName"].(string), aws.ToString(out.Stacks[0].StackName))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_CloudFormation_CustomResource exercises CFN extensibility types
// (Custom::*, WaitConditionHandle/WaitCondition, Macro) via a Terraform-managed stack,
// verifying that Gopherstack correctly handles all three resource classes end-to-end.
func TestTerraform_CloudFormation_CustomResource(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "custom_resource_round_trip",
			fixture: "cloudformation/custom_resource",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				id := uuid.NewString()[:8]

				return map[string]any{
					"StackName": "tf-cfn-ext-" + id,
					"MacroName": "TfTestMacro" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createCloudFormationClient(t)
				stackName := vars["StackName"].(string)

				out, err := client.DescribeStacks(ctx, &cfnsvc.DescribeStacksInput{
					StackName: aws.String(stackName),
				})
				require.NoError(t, err, "DescribeStacks should succeed after terraform apply")
				require.Len(t, out.Stacks, 1)

				s := out.Stacks[0]
				assert.Equal(t, stackName, aws.ToString(s.StackName))
				assert.Equal(t, "CREATE_COMPLETE", string(s.StackStatus),
					"stack with CustomResource + WaitCondition + Macro should reach CREATE_COMPLETE")

				// Verify Macro output is present.
				macroName := vars["MacroName"].(string)
				var foundMacro bool

				for _, o := range s.Outputs {
					if aws.ToString(o.OutputKey) == "MacroName" {
						assert.Equal(t, macroName, aws.ToString(o.OutputValue),
							"Macro physical ID should match registered macro name")
						foundMacro = true
					}
				}

				assert.True(t, foundMacro, "MacroName output should exist in stack outputs")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_ElastiCache provisions an ElastiCache cluster and verifies it exists.
func TestTerraform_ElastiCache(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "elasticache/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"ClusterID": "tf-ec-" + uuid.NewString()[:8]}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createElastiCacheClient(t)
				out, err := client.DescribeCacheClusters(ctx, &elasticachesvc.DescribeCacheClustersInput{
					CacheClusterId: aws.String(vars["ClusterID"].(string)),
				})
				require.NoError(t, err, "DescribeCacheClusters should succeed after terraform apply")
				require.Len(t, out.CacheClusters, 1)
				assert.Equal(t, vars["ClusterID"].(string), aws.ToString(out.CacheClusters[0].CacheClusterId))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_OpenSearch provisions an OpenSearch domain and verifies it exists.
func TestTerraform_OpenSearch(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "opensearch/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"DomainName": "tf-os-" + uuid.NewString()[:8]}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createOpenSearchClient(t)
				out, err := client.DescribeDomain(ctx, &opensearchsvc.DescribeDomainInput{
					DomainName: aws.String(vars["DomainName"].(string)),
				})
				require.NoError(t, err, "DescribeDomain should succeed after terraform apply")
				require.NotNil(t, out.DomainStatus)
				assert.Equal(t, vars["DomainName"].(string), aws.ToString(out.DomainStatus.DomainName))
				// The terraform-provider-aws create waiter settles when the domain
				// reports Created && !Processing with DomainProcessingStatus Active.
				assert.True(t, aws.ToBool(out.DomainStatus.Created), "domain should report Created")
				assert.False(t, aws.ToBool(out.DomainStatus.Processing), "settled domain must not be Processing")
				assert.Equal(t, "Active", string(out.DomainStatus.DomainProcessingStatus))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Elasticsearch provisions an Elasticsearch domain and verifies it exists.
func TestTerraform_Elasticsearch(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "elasticsearch/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"DomainName": "tf-es-" + uuid.NewString()[:8]}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createElasticsearchClient(t)
				out, err := client.DescribeElasticsearchDomain(ctx, &elasticsearchsvc.DescribeElasticsearchDomainInput{
					DomainName: aws.String(vars["DomainName"].(string)),
				})
				require.NoError(t, err, "DescribeElasticsearchDomain should succeed after terraform apply")
				require.NotNil(t, out.DomainStatus)
				assert.Equal(t, vars["DomainName"].(string), aws.ToString(out.DomainStatus.DomainName))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Redshift provisions a Redshift cluster and verifies it exists.
func TestTerraform_Redshift(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "redshift/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"ClusterID": "tf-rs-" + uuid.NewString()[:8]}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createRedshiftClient(t)
				out, err := client.DescribeClusters(ctx, &redshiftsvc.DescribeClustersInput{
					ClusterIdentifier: aws.String(vars["ClusterID"].(string)),
				})
				require.NoError(t, err, "DescribeClusters should succeed after terraform apply")
				require.Len(t, out.Clusters, 1)
				assert.Equal(t, vars["ClusterID"].(string), aws.ToString(out.Clusters[0].ClusterIdentifier))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Firehose provisions a Firehose delivery stream and verifies it exists.
func TestTerraform_Firehose(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "firehose/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"StreamName": "tf-fh-" + id,
					"RoleName":   "tf-fh-role-" + id,
					"BucketName": "tf-fh-bucket-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createFirehoseClient(t)
				out, err := client.DescribeDeliveryStream(ctx, &firehosesvc.DescribeDeliveryStreamInput{
					DeliveryStreamName: aws.String(vars["StreamName"].(string)),
				})
				require.NoError(t, err, "DescribeDeliveryStream should succeed after terraform apply")
				require.NotNil(t, out.DeliveryStreamDescription)
				assert.Equal(
					t,
					vars["StreamName"].(string),
					aws.ToString(out.DeliveryStreamDescription.DeliveryStreamName),
				)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_SWF provisions an SWF domain and verifies it is listed.
func TestTerraform_SWF(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "swf/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"DomainName": "tf-swf-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createSWFClient(t)
				out, err := client.ListDomains(ctx, &swfsvc.ListDomainsInput{
					RegistrationStatus: swftypes.RegistrationStatusRegistered,
				})
				require.NoError(t, err, "ListDomains should succeed after terraform apply")
				found := false
				for _, d := range out.DomainInfos {
					if aws.ToString(d.Name) == vars["DomainName"].(string) {
						found = true

						break
					}
				}
				assert.True(t, found, "SWF domain %q should be listed", vars["DomainName"].(string))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_ResourceGroups provisions a resource group and verifies it exists.
func TestTerraform_ResourceGroups(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "resourcegroups/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"GroupName": "tf-rg-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createResourceGroupsClient(t)
				out, err := client.GetGroup(ctx, &resourcegroupssvc.GetGroupInput{
					Group: aws.String(vars["GroupName"].(string)),
				})
				require.NoError(t, err, "GetGroup should succeed after terraform apply")
				require.NotNil(t, out.Group)
				assert.Equal(t, vars["GroupName"].(string), aws.ToString(out.Group.Name))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_ResourceGroupsTagging calls GetResources via the Resource Groups Tagging API data source.
func TestTerraform_ResourceGroupsTagging(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "get_resources",
			fixture: "resourcegroupstaggingapi/get_resources",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{}
			},
			verify: func(t *testing.T, ctx context.Context, _ map[string]any) {
				t.Helper()
				client := createResourceGroupsTaggingAPIClient(t)
				out, err := client.GetResources(ctx, &taggingsvc.GetResourcesInput{})
				require.NoError(t, err, "GetResources should succeed after terraform apply")
				require.NotNil(t, out)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_S3Control provisions an S3 account public access block and verifies it.
func TestTerraform_S3Control(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "s3control/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{}
			},
			verify: func(t *testing.T, ctx context.Context, _ map[string]any) {
				t.Helper()
				client := createS3ControlClient(t)
				out, err := client.GetPublicAccessBlock(ctx, &s3controlsvc.GetPublicAccessBlockInput{
					AccountId: aws.String("000000000000"),
				})
				require.NoError(t, err, "GetPublicAccessBlock should succeed after terraform apply")
				require.NotNil(t, out.PublicAccessBlockConfiguration)
				assert.True(t, aws.ToBool(out.PublicAccessBlockConfiguration.BlockPublicAcls))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_S3Website provisions an S3 bucket with website configuration and verifies it.
func TestTerraform_S3Website(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "s3_website/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"BucketName": "tf-s3-web-" + uuid.NewString()[:8]}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createS3Client(t)
				out, err := client.GetBucketWebsite(ctx, &s3svc.GetBucketWebsiteInput{
					Bucket: aws.String(vars["BucketName"].(string)),
				})
				require.NoError(t, err, "GetBucketWebsite should succeed after terraform apply")
				require.NotNil(t, out.IndexDocument)
				assert.Equal(t, "index.html", aws.ToString(out.IndexDocument.Suffix))
				require.NotNil(t, out.ErrorDocument)
				assert.Equal(t, "error.html", aws.ToString(out.ErrorDocument.Key))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_S3Encryption provisions an S3 bucket with server-side encryption configuration and verifies it.
func TestTerraform_S3Encryption(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "s3_encryption/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{"BucketName": "tf-s3-enc-" + uuid.NewString()[:8]}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createS3Client(t)
				out, err := client.GetBucketEncryption(ctx, &s3svc.GetBucketEncryptionInput{
					Bucket: aws.String(vars["BucketName"].(string)),
				})
				require.NoError(t, err, "GetBucketEncryption should succeed after terraform apply")
				require.NotNil(t, out.ServerSideEncryptionConfiguration)
				require.NotEmpty(t, out.ServerSideEncryptionConfiguration.Rules)
				require.NotNil(t, out.ServerSideEncryptionConfiguration.Rules[0].ApplyServerSideEncryptionByDefault)
				assert.Equal(
					t,
					s3types.ServerSideEncryptionAes256,
					out.ServerSideEncryptionConfiguration.Rules[0].ApplyServerSideEncryptionByDefault.SSEAlgorithm,
				)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_AWSConfig provisions a config recorder and delivery channel and verifies them.
func TestTerraform_AWSConfig(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "awsconfig/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"RoleName":     "tf-cfg-role-" + id,
					"BucketName":   "tf-cfg-bucket-" + id,
					"RecorderName": "tf-cfg-recorder-" + id,
					"ChannelName":  "tf-cfg-channel-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createAWSConfigClient(t)
				out, err := client.DescribeConfigurationRecorders(ctx, &configsvc.DescribeConfigurationRecordersInput{
					ConfigurationRecorderNames: []string{vars["RecorderName"].(string)},
				})
				require.NoError(t, err, "DescribeConfigurationRecorders should succeed after terraform apply")
				require.Len(t, out.ConfigurationRecorders, 1)
				assert.Equal(t, vars["RecorderName"].(string), aws.ToString(out.ConfigurationRecorders[0].Name))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Route53Resolver provisions a resolver rule and verifies it exists.
func TestTerraform_Route53Resolver(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "route53resolver/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"RuleName":     "tf-r53r-" + id,
					"EndpointName": "tf-r53r-ep-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createRoute53ResolverClient(t)
				out, err := client.ListResolverRules(ctx, &route53resolversvc.ListResolverRulesInput{})
				require.NoError(t, err, "ListResolverRules should succeed after terraform apply")
				found := false
				for _, r := range out.ResolverRules {
					if aws.ToString(r.Name) == vars["RuleName"].(string) {
						found = true

						break
					}
				}
				assert.True(t, found, "resolver rule %q should be listed", vars["RuleName"].(string))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_EC2 provisions a VPC, subnet, security group, and instance, then verifies the instance.
func TestTerraform_EC2(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "network_interface",
			fixture: "ec2/network_interface",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				vars := vpcCIDRVars(t)
				vars["ENIDescription"] = "tf-eni-" + uuid.NewString()[:8]

				return vars
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createEC2Client(t)

				// Verify the network interface exists.
				descOut, err := client.DescribeNetworkInterfaces(ctx, &ec2svc.DescribeNetworkInterfacesInput{})
				require.NoError(t, err, "DescribeNetworkInterfaces should succeed after terraform apply")

				eniDesc := vars["ENIDescription"].(string)
				var found bool

				for _, eni := range descOut.NetworkInterfaces {
					if aws.ToString(eni.Description) == eniDesc {
						found = true

						break
					}
				}

				require.True(t, found, "network interface with description %q should exist", eniDesc)
			},
		},
		{
			name:    "success",
			fixture: "ec2/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				vars := vpcCIDRVars(t)
				vars["SGName"] = "tf-ec2-sg-" + id

				return vars
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createEC2Client(t)

				// Verify the security group with the unique name was created.
				sgOut, err := client.DescribeSecurityGroups(ctx, &ec2svc.DescribeSecurityGroupsInput{})
				require.NoError(t, err, "DescribeSecurityGroups should succeed after terraform apply")
				sgName := vars["SGName"].(string)
				var found bool
				for _, sg := range sgOut.SecurityGroups {
					if aws.ToString(sg.GroupName) == sgName {
						found = true

						break
					}
				}
				require.True(t, found, "security group %q should exist", sgName)

				// Verify an instance was created.
				out, err := client.DescribeInstances(ctx, &ec2svc.DescribeInstancesInput{})
				require.NoError(t, err, "DescribeInstances should succeed after terraform apply")
				require.NotEmpty(t, out.Reservations, "at least one reservation should exist")
				require.NotEmpty(t, out.Reservations[0].Instances, "at least one instance should exist")

				// Verify that tags from the fixture's `tags = {}` blocks were stored
				// (via TagSpecification on CreateVpc / standalone CreateTags).
				// Find the VPC created by this fixture (CIDR is per-test, see vpcCIDRVars).
				vpcCidr := vars["VPCCidr"].(string)

				vpcsOut, err := client.DescribeVpcs(ctx, &ec2svc.DescribeVpcsInput{})
				require.NoError(t, err, "DescribeVpcs should succeed after terraform apply")

				var vpcID string
				for _, vpc := range vpcsOut.Vpcs {
					if aws.ToString(vpc.CidrBlock) == vpcCidr && !aws.ToBool(vpc.IsDefault) {
						vpcID = aws.ToString(vpc.VpcId)

						break
					}
				}

				require.NotEmpty(t, vpcID, "VPC with CIDR %s should exist after terraform apply", vpcCidr)

				// DescribeTags with resource-id filter to get tags for the fixture's VPC.
				tagsOut, err := client.DescribeTags(ctx, &ec2svc.DescribeTagsInput{
					Filters: []ec2types.Filter{
						{Name: aws.String("resource-id"), Values: []string{vpcID}},
					},
				})
				require.NoError(t, err, "DescribeTags should succeed after terraform apply")

				vpcTags := make(map[string]string)
				for _, tag := range tagsOut.Tags {
					vpcTags[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
				}

				assert.Equal(t, "test-vpc", vpcTags["Name"], "VPC should have Name=test-vpc tag from terraform fixture")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_ECRLambda provisions an ECR repository and a Lambda function
// wired to use that ECR repository URI, then invokes the Lambda and confirms
// the response reflects the repository URI stored in the environment variable.
func TestTerraform_ECRLambda(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "ecr_lambda/success",
			setup: func(t *testing.T, dir string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				// Build a minimal zip containing a Python handler that returns
				// the ECR_REPO_URI environment variable.
				var buf bytes.Buffer
				zw := zip.NewWriter(&buf)
				f, err := zw.Create("index.py")
				require.NoError(t, err)
				_, err = f.Write([]byte(
					"import os, json\n" +
						"def handler(event, context):\n" +
						"    return {\"ecr_repo_url\": os.environ.get(\"ECR_REPO_URL\", \"\")}\n",
				))
				require.NoError(t, err)
				require.NoError(t, zw.Close())

				zipPath := filepath.Join(dir, "function.zip")
				require.NoError(t, os.WriteFile(zipPath, buf.Bytes(), 0o644))

				return map[string]any{
					"RepoName": "tf-ecr-repo-" + id,
					"FuncName": "tf-ecr-lambda-" + id,
					"RoleName": "tf-ecr-role-" + id,
					"ZipPath":  zipPath,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				// 1. Verify the ECR repository was created.
				ecrClient := createECRClient(t)
				repoOut, err := ecrClient.DescribeRepositories(ctx, &ecrsvc.DescribeRepositoriesInput{
					RepositoryNames: []string{vars["RepoName"].(string)},
				})
				require.NoError(t, err, "ECR DescribeRepositories should succeed")
				require.Len(t, repoOut.Repositories, 1)

				repoURI := aws.ToString(repoOut.Repositories[0].RepositoryUri)
				assert.Contains(t, repoURI, vars["RepoName"].(string))

				// 2. Verify the Lambda function was created with the ECR image URI in its env.
				lambdaClient := createLambdaClient(t)
				fnOut, err := lambdaClient.GetFunction(ctx, &lambdasvc.GetFunctionInput{
					FunctionName: aws.String(vars["FuncName"].(string)),
				})
				require.NoError(t, err, "GetFunction should succeed")
				require.NotNil(t, fnOut.Configuration)
				assert.Equal(t, vars["FuncName"].(string), aws.ToString(fnOut.Configuration.FunctionName))

				// 3. Confirm the ECR repo URI is wired into the Lambda environment.
				// This validates the cross-service Terraform wiring: ECR → Lambda env var.
				require.NotNil(t, fnOut.Configuration.Environment)
				envVars := fnOut.Configuration.Environment.Variables
				assert.Contains(t, envVars["ECR_REPO_URL"], vars["RepoName"].(string),
					"Lambda ECR_REPO_URL env var should contain ECR repo name")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_APIGateway provisions a REST API, resource, method, integration, and deployment.
func TestTerraform_APIGateway(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "apigateway/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"APIName": "tf-apigw-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createAPIGatewayClient(t)
				out, err := client.GetRestApis(ctx, &apigwsvc.GetRestApisInput{})
				require.NoError(t, err, "GetRestApis should succeed after terraform apply")
				found := false
				for _, api := range out.Items {
					if aws.ToString(api.Name) == vars["APIName"].(string) {
						found = true

						break
					}
				}
				assert.True(t, found, "REST API %q should be listed", vars["APIName"].(string))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_APIGateway_DataPlane provisions a REST API with a MOCK integration via Terraform
// and verifies that a live HTTP request routed through the API Gateway data plane returns HTTP 200.
// MOCK integration is used so this test runs inside the scratch-based container image (no /tmp or
// Docker-in-Docker required for Lambda execution).
func TestTerraform_APIGateway_DataPlane(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "mock_integration",
			fixture: "apigateway/proxy",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"APIName": "tf-apigw-dp-api-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				// Look up the deployed API ID via the AWS SDK.
				apiClient := createAPIGatewayClient(t)
				apis, err := apiClient.GetRestApis(ctx, &apigwsvc.GetRestApisInput{})
				require.NoError(t, err)

				var apiID string
				for _, api := range apis.Items {
					if aws.ToString(api.Name) == vars["APIName"].(string) {
						apiID = aws.ToString(api.Id)

						break
					}
				}
				require.NotEmpty(t, apiID, "REST API %q should be present", vars["APIName"].(string))

				// Invoke the deployed API through the data-plane endpoint.
				// The MOCK integration returns HTTP 200 with an empty body by default.
				url := endpoint + "/restapis/" + apiID + "/prod/_user_request_/items"
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
				require.NoError(t, err)

				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err)

				defer resp.Body.Close()

				assert.Equal(t, http.StatusOK, resp.StatusCode,
					"data-plane request to /restapis/%s/prod/_user_request_/items should return 200", apiID)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_APIGateway_RequestValidation provisions a REST API whose GET method
// declares a required querystring parameter and a request validator, then confirms the
// data plane rejects a request missing the parameter with HTTP 400 and accepts one that
// includes it with HTTP 200. Mirrors terraform-provider-aws's
// aws_api_gateway_request_validator acceptance coverage, extended to runtime behavior.
func TestTerraform_APIGateway_RequestValidation(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "required_query_param",
			fixture: "apigateway/request_validation",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"APIName": "tf-apigw-rv-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				apiClient := createAPIGatewayClient(t)
				apis, err := apiClient.GetRestApis(ctx, &apigwsvc.GetRestApisInput{})
				require.NoError(t, err)

				var apiID string
				for _, api := range apis.Items {
					if aws.ToString(api.Name) == vars["APIName"].(string) {
						apiID = aws.ToString(api.Id)

						break
					}
				}
				require.NotEmpty(t, apiID, "REST API %q should be present", vars["APIName"].(string))

				base := endpoint + "/restapis/" + apiID + "/prod/_user_request_/items"

				// Missing required query parameter → 400.
				missReq, err := http.NewRequestWithContext(ctx, http.MethodGet, base, nil)
				require.NoError(t, err)
				missResp, err := http.DefaultClient.Do(missReq)
				require.NoError(t, err)
				defer missResp.Body.Close()
				assert.Equal(t, http.StatusBadRequest, missResp.StatusCode,
					"request missing required query param must be rejected with 400")

				// Required query parameter present → 200.
				okReq, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?q=hello", nil)
				require.NoError(t, err)
				okResp, err := http.DefaultClient.Do(okReq)
				require.NoError(t, err)
				defer okResp.Body.Close()
				assert.Equal(t, http.StatusOK, okResp.StatusCode,
					"request with required query param must be accepted with 200")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Scheduler provisions an EventBridge Scheduler schedule and verifies it exists.
func TestTerraform_Scheduler(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "scheduler/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"RoleName":     "tf-sched-role-" + id,
					"ScheduleName": "tf-sched-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createSchedulerClient(t)
				out, err := client.GetSchedule(ctx, &schedulersvc.GetScheduleInput{
					Name: aws.String(vars["ScheduleName"].(string)),
				})
				require.NoError(t, err, "GetSchedule should succeed after terraform apply")
				assert.Equal(t, vars["ScheduleName"].(string), aws.ToString(out.Name))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_AppSync provisions an AppSync GraphQL API and a NONE data source, then verifies
// both exist via the AppSync SDK.
func TestTerraform_AppSync(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "api_and_datasource",
			fixture: "appsync/api_and_datasource",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{"APIName": "tf-appsync-" + id}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createAppSyncClient(t)

				// List APIs and find ours by name.
				listOut, err := client.ListGraphqlApis(ctx, &appsyncsdkv2.ListGraphqlApisInput{})
				require.NoError(t, err, "ListGraphqlApis should succeed")

				var apiID string
				for _, a := range listOut.GraphqlApis {
					if aws.ToString(a.Name) == vars["APIName"].(string) {
						apiID = aws.ToString(a.ApiId)

						break
					}
				}

				require.NotEmpty(t, apiID, "API %q should appear in list", vars["APIName"])
				assert.Equal(t, appsyncsdktypes.AuthenticationTypeApiKey, listOut.GraphqlApis[0].AuthenticationType)

				// Verify data source exists.
				dsOut, err := client.GetDataSource(ctx, &appsyncsdkv2.GetDataSourceInput{
					ApiId: aws.String(apiID),
					Name:  aws.String("NoneDS"),
				})
				require.NoError(t, err, "GetDataSource should succeed")
				assert.Equal(t, "NoneDS", aws.ToString(dsOut.DataSource.Name))
				assert.Equal(t, appsyncsdktypes.DataSourceTypeNone, dsOut.DataSource.Type)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_APIGatewayV2 provisions an HTTP API, stage, integration, and route
// via Terraform, then verifies the API is listed via the API Gateway V2 SDK.
func TestTerraform_APIGatewayV2(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "apigatewayv2/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"APIName": "tf-apigwv2-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createAPIGatewayV2Client(t)
				out, err := client.GetApis(ctx, &apigwv2svc.GetApisInput{})
				require.NoError(t, err, "GetApis should succeed after terraform apply")
				found := false
				for _, api := range out.Items {
					if aws.ToString(api.Name) == vars["APIName"].(string) {
						found = true

						break
					}
				}
				assert.True(t, found, "HTTP API %q should be listed", vars["APIName"].(string))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Autoscaling provisions a launch configuration and Auto Scaling group via Terraform,
// then verifies both exist via the Autoscaling SDK.
func TestTerraform_Autoscaling(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "autoscaling/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				suffix := uuid.NewString()[:8]

				return map[string]any{
					"ASGName": "tf-asg-" + suffix,
					"LCName":  "tf-lc-" + suffix,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createAutoscalingClient(t)

				out, err := client.DescribeAutoScalingGroups(ctx, &autoscalingsvc.DescribeAutoScalingGroupsInput{
					AutoScalingGroupNames: []string{vars["ASGName"].(string)},
				})
				require.NoError(t, err, "DescribeAutoScalingGroups should succeed after terraform apply")
				require.Len(t, out.AutoScalingGroups, 1)
				assert.Equal(t, vars["ASGName"].(string), aws.ToString(out.AutoScalingGroups[0].AutoScalingGroupName))
				assert.Equal(t, int32(1), aws.ToInt32(out.AutoScalingGroups[0].MinSize))
				assert.Equal(t, int32(5), aws.ToInt32(out.AutoScalingGroups[0].MaxSize))

				lcOut, err := client.DescribeLaunchConfigurations(
					ctx,
					&autoscalingsvc.DescribeLaunchConfigurationsInput{
						LaunchConfigurationNames: []string{vars["LCName"].(string)},
					},
				)
				require.NoError(t, err, "DescribeLaunchConfigurations should succeed")
				require.Len(t, lcOut.LaunchConfigurations, 1)
				assert.Equal(
					t,
					vars["LCName"].(string),
					aws.ToString(lcOut.LaunchConfigurations[0].LaunchConfigurationName),
				)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Amplify provisions an Amplify app and branch via Terraform,
// then verifies both exist via the Amplify SDK.
func TestTerraform_Amplify(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "app_and_branch",
			fixture: "amplify/app_and_branch",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				randomSuffix := uuid.NewString()[:8]

				return map[string]any{"AppName": "tf-amplify-" + randomSuffix}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createAmplifyClient(t)

				// List apps and find ours by name.
				listOut, err := client.ListApps(ctx, &amplifysdkv2.ListAppsInput{})
				require.NoError(t, err, "ListApps should succeed")

				var appID string
				for _, a := range listOut.Apps {
					if aws.ToString(a.Name) == vars["AppName"].(string) {
						appID = aws.ToString(a.AppId)

						break
					}
				}

				require.NotEmpty(t, appID, "app %q should appear in list", vars["AppName"])

				// Verify the branch exists.
				branchOut, err := client.GetBranch(ctx, &amplifysdkv2.GetBranchInput{
					AppId:      aws.String(appID),
					BranchName: aws.String("main"),
				})
				require.NoError(t, err, "GetBranch should succeed")
				assert.Equal(t, "main", aws.ToString(branchOut.Branch.BranchName))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_ECS provisions an ECS cluster, task definition, and service via Terraform
// and verifies they exist via the ECS SDK.
func TestTerraform_ECS(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "ecs/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"ClusterName": "tf-ecs-cluster-" + id,
					"Family":      "tf-ecs-family-" + id,
					"ServiceName": "tf-ecs-service-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createECSClient(t)

				// Verify cluster exists.
				clusterOut, err := client.DescribeClusters(ctx, &ecssvc.DescribeClustersInput{
					Clusters: []string{vars["ClusterName"].(string)},
				})
				require.NoError(t, err, "DescribeClusters should succeed after terraform apply")
				require.Len(t, clusterOut.Clusters, 1)
				assert.Equal(t, vars["ClusterName"].(string), *clusterOut.Clusters[0].ClusterName)

				// Verify task definition exists.
				tdOut, err := client.DescribeTaskDefinition(ctx, &ecssvc.DescribeTaskDefinitionInput{
					TaskDefinition: aws.String(vars["Family"].(string)),
				})
				require.NoError(t, err, "DescribeTaskDefinition should succeed after terraform apply")
				assert.Equal(t, vars["Family"].(string), *tdOut.TaskDefinition.Family)

				// Verify service exists.
				svcOut, err := client.DescribeServices(ctx, &ecssvc.DescribeServicesInput{
					Cluster:  aws.String(vars["ClusterName"].(string)),
					Services: []string{vars["ServiceName"].(string)},
				})
				require.NoError(t, err, "DescribeServices should succeed after terraform apply")
				require.Len(t, svcOut.Services, 1)
				assert.Equal(t, vars["ServiceName"].(string), *svcOut.Services[0].ServiceName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_CognitoIdentityPool provisions a Cognito Identity Pool with IAM roles
// and a roles attachment via Terraform, then verifies the pool and its roles exist.
func TestTerraform_CognitoIdentityPool(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "cognitoidentity/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				id := uuid.NewString()[:8]

				return map[string]any{
					"PoolName":       "tf-identity-pool-" + id,
					"AuthRoleName":   "tf-cognito-auth-" + id,
					"UnauthRoleName": "tf-cognito-unauth-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createCognitoIdentityClient(t)

				// List pools and find ours by name.
				listOut, err := client.ListIdentityPools(ctx, &cognitoidentitysvc.ListIdentityPoolsInput{
					MaxResults: aws.Int32(60),
				})
				require.NoError(t, err, "ListIdentityPools should succeed after terraform apply")

				var poolID string

				for _, p := range listOut.IdentityPools {
					if aws.ToString(p.IdentityPoolName) == vars["PoolName"].(string) {
						poolID = aws.ToString(p.IdentityPoolId)

						break
					}
				}

				require.NotEmpty(t, poolID, "identity pool %q should appear in list", vars["PoolName"])

				// Describe pool to confirm it exists.
				descOut, err := client.DescribeIdentityPool(ctx, &cognitoidentitysvc.DescribeIdentityPoolInput{
					IdentityPoolId: aws.String(poolID),
				})
				require.NoError(t, err, "DescribeIdentityPool should succeed after terraform apply")
				assert.Equal(t, vars["PoolName"].(string), aws.ToString(descOut.IdentityPoolName))
				assert.True(t, descOut.AllowUnauthenticatedIdentities)

				// Verify roles were attached via GetIdentityPoolRoles.
				rolesOut, err := client.GetIdentityPoolRoles(ctx, &cognitoidentitysvc.GetIdentityPoolRolesInput{
					IdentityPoolId: aws.String(poolID),
				})
				require.NoError(t, err, "GetIdentityPoolRoles should succeed after terraform apply")
				assert.NotEmpty(t, rolesOut.Roles["authenticated"], "authenticated role ARN should be set")
				assert.NotEmpty(t, rolesOut.Roles["unauthenticated"], "unauthenticated role ARN should be set")
				assert.Contains(t, rolesOut.Roles["authenticated"], vars["AuthRoleName"].(string))
				assert.Contains(t, rolesOut.Roles["unauthenticated"], vars["UnauthRoleName"].(string))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_SESv2 provisions SES v2 email identity and configuration set resources.
func TestTerraform_SESv2(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "sesv2/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{
					"Email":         "tf-sesv2-" + uuid.NewString()[:8] + "@example.com",
					"ConfigSetName": "tf-cfg-" + uuid.NewString()[:8],
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				email := vars["Email"].(string)
				cfgName := vars["ConfigSetName"].(string)

				// Verify email identity was created.
				req, err := http.NewRequestWithContext(ctx, http.MethodGet,
					endpoint+"/v2/email/identities/"+email, nil)
				require.NoError(t, err)

				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err)
				defer resp.Body.Close()

				assert.Equal(t, http.StatusOK, resp.StatusCode,
					"email identity %q should exist after terraform apply", email)

				var identityOut map[string]any
				require.NoError(t, json.NewDecoder(resp.Body).Decode(&identityOut))
				assert.Equal(t, email, identityOut["EmailIdentity"])

				// Verify configuration set was created.
				req2, err := http.NewRequestWithContext(ctx, http.MethodGet,
					endpoint+"/v2/email/configuration-sets/"+cfgName, nil)
				require.NoError(t, err)

				resp2, err := http.DefaultClient.Do(req2)
				require.NoError(t, err)
				defer resp2.Body.Close()

				assert.Equal(t, http.StatusOK, resp2.StatusCode,
					"configuration set %q should exist after terraform apply", cfgName)

				var cfgOut map[string]any
				require.NoError(t, json.NewDecoder(resp2.Body).Decode(&cfgOut))
				assert.Equal(t, cfgName, cfgOut["ConfigurationSetName"])
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_CognitoIDP provisions a Cognito User Pool and User Pool Client via Terraform
// and verifies both exist via the Cognito IDP SDK: ListUserPools + DescribeUserPool for the
// pool, and ListUserPoolClients + DescribeUserPoolClient for the client.
func TestTerraform_CognitoIDP(t *testing.T) {
	t.Parallel()

	const maxUserPoolsToList = int32(60)

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "cognitoidp/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"PoolName":   "tf-pool-" + id,
					"ClientName": "tf-client-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				sdkClient := createCognitoIDPClient(t)

				// List pools and find ours by name.
				listOut, err := sdkClient.ListUserPools(ctx, &cognitoidpsvc.ListUserPoolsInput{
					MaxResults: aws.Int32(maxUserPoolsToList),
				})
				require.NoError(t, err, "ListUserPools should succeed")

				var poolID string

				for _, p := range listOut.UserPools {
					if aws.ToString(p.Name) == vars["PoolName"].(string) {
						poolID = aws.ToString(p.Id)

						break
					}
				}

				require.NotEmpty(t, poolID, "user pool %q should appear in list", vars["PoolName"])

				// Describe the pool.
				descOut, err := sdkClient.DescribeUserPool(ctx, &cognitoidpsvc.DescribeUserPoolInput{
					UserPoolId: aws.String(poolID),
				})
				require.NoError(t, err, "DescribeUserPool should succeed")
				assert.Equal(t, vars["PoolName"].(string), aws.ToString(descOut.UserPool.Name))

				// List clients for the pool and find ours by name.
				clientsOut, err := sdkClient.ListUserPoolClients(ctx, &cognitoidpsvc.ListUserPoolClientsInput{
					UserPoolId: aws.String(poolID),
					MaxResults: aws.Int32(maxUserPoolsToList),
				})
				require.NoError(t, err, "ListUserPoolClients should succeed")

				var clientID string

				for _, c := range clientsOut.UserPoolClients {
					if aws.ToString(c.ClientName) == vars["ClientName"].(string) {
						clientID = aws.ToString(c.ClientId)

						break
					}
				}

				require.NotEmpty(t, clientID, "user pool client %q should appear in list", vars["ClientName"])

				// Describe the client.
				descClientOut, err := sdkClient.DescribeUserPoolClient(ctx, &cognitoidpsvc.DescribeUserPoolClientInput{
					UserPoolId: aws.String(poolID),
					ClientId:   aws.String(clientID),
				})
				require.NoError(t, err, "DescribeUserPoolClient should succeed")
				assert.Equal(t, vars["ClientName"].(string), aws.ToString(descClientOut.UserPoolClient.ClientName))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_IoTDataPlane provisions an IoT thing via Terraform and verifies
// that the IoT Data Plane can publish a message to the thing's shadow topic.
func TestTerraform_IoTDataPlane(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "publish",
			fixture: "iotdataplane/publish",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{"ThingName": "tf-iot-thing-" + id}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				thingName := vars["ThingName"].(string)
				topic := "things/" + thingName + "/shadow/update"
				url := endpoint + "/topics/" + topic

				payload := []byte(`{"state":{"reported":{"connected":true}}}`)

				req, err := http.NewRequestWithContext(
					ctx,
					http.MethodPost,
					url,
					bytes.NewReader(payload),
				)
				require.NoError(t, err, "creating publish request should succeed")
				req.Header.Set("Content-Type", "application/json")

				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err, "publish request should succeed")
				defer resp.Body.Close()

				assert.Equal(t, http.StatusOK, resp.StatusCode,
					"IoT Data Plane publish to topic %q should return 200", topic)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_IoT provisions an IoT Thing via Terraform and verifies it exists via the IoT SDK.
func TestTerraform_IoT(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "iot/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{"ThingName": "tf-iot-thing-" + id}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createIoTClient(t)

				out, err := client.DescribeThing(ctx, &iotsvc.DescribeThingInput{
					ThingName: aws.String(vars["ThingName"].(string)),
				})
				require.NoError(t, err, "DescribeThing should succeed after terraform apply")
				assert.Equal(t, vars["ThingName"].(string), aws.ToString(out.ThingName))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_STS verifies that the aws_caller_identity data source works via
// the STS GetCallerIdentity operation.
func TestTerraform_STS(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "sts/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{}
			},
			verify: func(t *testing.T, ctx context.Context, _ map[string]any) {
				t.Helper()

				client := createSTSClient(t)

				out, err := client.GetCallerIdentity(ctx, &stssvc.GetCallerIdentityInput{})
				require.NoError(t, err, "GetCallerIdentity should succeed after terraform apply")
				require.NotNil(t, out)
				assert.Equal(
					t,
					"000000000000",
					aws.ToString(out.Account),
					"mock STS should return the fixed test account ID",
				)
				assert.Contains(
					t,
					aws.ToString(out.Arn),
					":root",
					"mock STS ARN should be a root identity for static credentials",
				)
				assert.NotEmpty(t, aws.ToString(out.UserId), "mock STS user ID should not be empty")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Support applies a Terraform fixture and verifies Support API CreateCase,
// DescribeCases, and ResolveCase operations via the AWS SDK.
func TestTerraform_Support(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "support/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{"CaseName": "tf-support-" + id}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createSupportClient(t)

				subject := vars["CaseName"].(string)

				createOut, err := client.CreateCase(ctx, &supportsvc.CreateCaseInput{
					Subject:           aws.String(subject),
					CommunicationBody: aws.String("Terraform integration test case"),
					ServiceCode:       aws.String("general-info"),
					CategoryCode:      aws.String("other"),
					SeverityCode:      aws.String("low"),
				})
				require.NoError(t, err, "CreateCase should succeed")
				require.NotNil(t, createOut.CaseId, "CreateCase should return a caseId")

				caseID := aws.ToString(createOut.CaseId)

				describeOut, err := client.DescribeCases(ctx, &supportsvc.DescribeCasesInput{
					CaseIdList: []string{caseID},
				})
				require.NoError(t, err, "DescribeCases should succeed")
				require.Len(t, describeOut.Cases, 1, "DescribeCases should return the created case")
				assert.Equal(t, subject, aws.ToString(describeOut.Cases[0].Subject))

				resolveOut, err := client.ResolveCase(ctx, &supportsvc.ResolveCaseInput{
					CaseId: aws.String(caseID),
				})
				require.NoError(t, err, "ResolveCase should succeed")
				assert.Equal(t, "resolved", aws.ToString(resolveOut.FinalCaseStatus))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_APIGatewayManagementAPI provisions an API Gateway REST API via Terraform
// and verifies that the API Gateway Management API endpoints work correctly.
func TestTerraform_APIGatewayManagementAPI(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "connections",
			fixture: "apigatewaymanagementapi/connections",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{"APIName": "tf-apigw-mgmt-" + id}
			},
			verify: func(t *testing.T, ctx context.Context, _ map[string]any) {
				t.Helper()

				connectionID := "tf-conn-" + uuid.NewString()[:8]

				// Create a simulated connection via the dashboard endpoint.
				createURL := endpoint + "/dashboard/apigatewaymanagementapi/connection/create"
				createReq, err := http.NewRequestWithContext(
					ctx,
					http.MethodPost,
					createURL,
					strings.NewReader("connectionId="+connectionID+"&sourceIp=10.0.0.1&userAgent=test-agent"),
				)
				require.NoError(t, err, "creating dashboard create request should succeed")
				createReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

				createClient := &http.Client{
					CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
						return http.ErrUseLastResponse
					},
				}
				createResp, err := createClient.Do(createReq)
				require.NoError(t, err, "dashboard create connection request should succeed")
				defer createResp.Body.Close()
				assert.Equal(t, http.StatusFound, createResp.StatusCode, "create connection should redirect")

				// GetConnection via /@connections/{connectionId}.
				getURL := endpoint + "/@connections/" + connectionID
				getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, getURL, nil)
				require.NoError(t, err, "creating get request should succeed")
				getReq.Header.Set("Authorization", "Bearer test")

				getResp, err := http.DefaultClient.Do(getReq)
				require.NoError(t, err, "GetConnection should succeed")
				defer getResp.Body.Close()
				assert.Equal(t, http.StatusOK, getResp.StatusCode, "GetConnection should return 200")

				// PostToConnection via /@connections/{connectionId}.
				postURL := endpoint + "/@connections/" + connectionID
				postReq, err := http.NewRequestWithContext(
					ctx,
					http.MethodPost,
					postURL,
					bytes.NewReader([]byte(`{"message":"hello from test"}`)),
				)
				require.NoError(t, err, "creating post request should succeed")
				postReq.Header.Set("Content-Type", "application/json")
				postReq.Header.Set("Authorization", "Bearer test")

				postResp, err := http.DefaultClient.Do(postReq)
				require.NoError(t, err, "PostToConnection should succeed")
				defer postResp.Body.Close()
				assert.Equal(t, http.StatusOK, postResp.StatusCode, "PostToConnection should return 200")

				// DeleteConnection via /@connections/{connectionId}.
				delURL := endpoint + "/@connections/" + connectionID
				delReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, delURL, nil)
				require.NoError(t, err, "creating delete request should succeed")
				delReq.Header.Set("Authorization", "Bearer test")

				delResp, err := http.DefaultClient.Do(delReq)
				require.NoError(t, err, "DeleteConnection should succeed")
				defer delResp.Body.Close()
				assert.Equal(t, http.StatusNoContent, delResp.StatusCode, "DeleteConnection should return 204")

				// GetConnection after delete should return 410.
				getReq2, err := http.NewRequestWithContext(ctx, http.MethodGet, getURL, nil)
				require.NoError(t, err, "creating second get request should succeed")
				getReq2.Header.Set("Authorization", "Bearer test")
				getResp2, err := http.DefaultClient.Do(getReq2)
				require.NoError(t, err, "GetConnection after delete should not error")
				defer getResp2.Body.Close()
				assert.Equal(t, http.StatusGone, getResp2.StatusCode, "GetConnection after delete should return 410")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_AppConfig provisions an AppConfig application, environment, configuration profile,
// and deployment strategy via Terraform, then verifies they are accessible via the AppConfig SDK.
func TestTerraform_AppConfig(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "app_env_profile_strategy",
			fixture: "appconfig/app_env_profile_strategy",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				randomSuffix := uuid.NewString()[:8]

				return map[string]any{"AppName": "tf-appconfig-" + randomSuffix}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createAppConfigClient(t)

				listOut, err := client.ListApplications(ctx, &appconfigsvc.ListApplicationsInput{})
				require.NoError(t, err, "ListApplications should succeed")

				appID := findAppConfigApplicationID(listOut.Items, vars["AppName"].(string))
				require.NotEmpty(t, appID, "application %q should appear in list", vars["AppName"])

				getOut, err := client.GetApplication(ctx, &appconfigsvc.GetApplicationInput{
					ApplicationId: aws.String(appID),
				})
				require.NoError(t, err, "GetApplication should succeed")
				assert.Equal(t, vars["AppName"].(string), aws.ToString(getOut.Name))

				envsOut, err := client.ListEnvironments(ctx, &appconfigsvc.ListEnvironmentsInput{
					ApplicationId: aws.String(appID),
				})
				require.NoError(t, err, "ListEnvironments should succeed")
				require.NotEmpty(t, envsOut.Items, "should have at least one environment")

				profilesOut, err := client.ListConfigurationProfiles(ctx, &appconfigsvc.ListConfigurationProfilesInput{
					ApplicationId: aws.String(appID),
				})
				require.NoError(t, err, "ListConfigurationProfiles should succeed")
				require.NotEmpty(t, profilesOut.Items, "should have at least one configuration profile")

				strategiesOut, err := client.ListDeploymentStrategies(
					ctx,
					&appconfigsvc.ListDeploymentStrategiesInput{},
				)
				require.NoError(t, err, "ListDeploymentStrategies should succeed")
				require.NotEmpty(t, strategiesOut.Items, "should have at least one deployment strategy")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// findAppConfigApplicationID returns the ID of the AppConfig application with the given name
// from the list output, or empty string if not found.
func findAppConfigApplicationID(items []appconfigtypes.Application, name string) string {
	for _, a := range items {
		if aws.ToString(a.Name) == name {
			return aws.ToString(a.Id)
		}
	}

	return ""
}

// TestTerraform_AppConfigData verifies that the AppConfigData service endpoints work correctly.
func TestTerraform_AppConfigData(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "session",
			fixture: "appconfigdata/session",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"AppName": "tf-app-" + id,
					"EnvName": "prod",
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				appID := vars["AppName"].(string)
				envID := vars["EnvName"].(string)
				profileID := "my-profile"
				configContent := `{"featureFlag":true,"version":"1.0"}`

				// Seed configuration via dashboard.
				setURL := endpoint + "/dashboard/appconfigdata/configuration/set"
				formData := strings.Join([]string{
					"applicationIdentifier=" + appID,
					"environmentIdentifier=" + envID,
					"configurationProfileIdentifier=" + profileID,
					"contentType=application%2Fjson",
					"content=" + strings.ReplaceAll(configContent, "\"", "%22"),
				}, "&")

				setReq, err := http.NewRequestWithContext(
					ctx,
					http.MethodPost,
					setURL,
					strings.NewReader(formData),
				)
				require.NoError(t, err, "creating set configuration request should succeed")
				setReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

				setClient := &http.Client{
					CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
						return http.ErrUseLastResponse
					},
				}
				setResp, err := setClient.Do(setReq)
				require.NoError(t, err, "set configuration request should succeed")
				defer setResp.Body.Close()
				assert.Equal(t, http.StatusFound, setResp.StatusCode, "set configuration should redirect")

				// Start a configuration session via the AppConfigData API.
				client := createAppConfigDataClient(t)

				sessionOut, err := client.StartConfigurationSession(
					ctx,
					&appconfigdatasvc.StartConfigurationSessionInput{
						ApplicationIdentifier:          aws.String(appID),
						EnvironmentIdentifier:          aws.String(envID),
						ConfigurationProfileIdentifier: aws.String(profileID),
					},
				)
				require.NoError(t, err, "StartConfigurationSession should succeed")
				require.NotNil(t, sessionOut.InitialConfigurationToken, "initial token should not be nil")
				assert.NotEmpty(t, *sessionOut.InitialConfigurationToken, "initial token should not be empty")

				// Get latest configuration.
				configOut, err := client.GetLatestConfiguration(
					ctx,
					&appconfigdatasvc.GetLatestConfigurationInput{
						ConfigurationToken: sessionOut.InitialConfigurationToken,
					},
				)
				require.NoError(t, err, "GetLatestConfiguration should succeed")
				require.NotNil(t, configOut, "config output should not be nil")

				assert.Contains(
					t,
					string(configOut.Configuration),
					"featureFlag",
					"configuration should contain expected content",
				)

				// Next token should rotate.
				assert.NotNil(t, configOut.NextPollConfigurationToken, "next token should not be nil")
				assert.NotEqual(t, *sessionOut.InitialConfigurationToken, *configOut.NextPollConfigurationToken,
					"next token should differ from initial token")

				// Poll again with the new token to verify token rotation.
				configOut2, err := client.GetLatestConfiguration(
					ctx,
					&appconfigdatasvc.GetLatestConfigurationInput{
						ConfigurationToken: configOut.NextPollConfigurationToken,
					},
				)
				require.NoError(t, err, "second GetLatestConfiguration should succeed")
				require.NotNil(t, configOut2, "second config output should not be nil")
				require.NotNil(t, configOut2.NextPollConfigurationToken, "second next token should not be nil")
				assert.NotEqual(t, *configOut.NextPollConfigurationToken, *configOut2.NextPollConfigurationToken,
					"token should rotate on each successive poll")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_ApplicationAutoscaling provisions a scalable target and verifies it exists.
func TestTerraform_ApplicationAutoscaling(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "applicationautoscaling/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{
					"ServiceName": "tf-svc-" + uuid.NewString()[:8],
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createApplicationAutoscalingClient(t)
				out, err := client.DescribeScalableTargets(ctx, &applicationautoscalingsvc.DescribeScalableTargetsInput{
					ServiceNamespace: applicationautoscalingtypes.ServiceNamespaceEcs,
				})
				require.NoError(t, err, "DescribeScalableTargets should succeed after terraform apply")

				expectedResourceID := "service/default/" + vars["ServiceName"].(string)
				found := false

				for _, target := range out.ScalableTargets {
					if aws.ToString(target.ResourceId) == expectedResourceID {
						found = true
						assert.Equal(t, int32(1), aws.ToInt32(target.MinCapacity))
						assert.Equal(t, int32(10), aws.ToInt32(target.MaxCapacity))

						break
					}
				}

				assert.True(t, found, "scalable target should be registered")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Athena provisions an Athena workgroup via Terraform, then verifies
// it is listed via the Athena SDK.
func TestTerraform_Athena(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "athena/workgroup",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"WorkGroupName": "tf-athena-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createAthenaClient(t)
				out, err := client.ListWorkGroups(ctx, &athenasdkv2.ListWorkGroupsInput{})
				require.NoError(t, err, "ListWorkGroups should succeed after terraform apply")
				found := false
				for _, wg := range out.WorkGroups {
					if aws.ToString(wg.Name) == vars["WorkGroupName"].(string) {
						found = true

						break
					}
				}
				assert.True(t, found, "workgroup %q should be listed", vars["WorkGroupName"].(string))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Backup provisions an AWS Backup vault via Terraform, then verifies
// it is listed via the Backup SDK.
func TestTerraform_Backup(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "backup/vault",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"VaultName": "tf-backup-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createBackupClient(t)
				out, err := client.ListBackupVaults(ctx, &backupsvc.ListBackupVaultsInput{})
				require.NoError(t, err, "ListBackupVaults should succeed after terraform apply")
				found := false
				for _, v := range out.BackupVaultList {
					if aws.ToString(v.BackupVaultName) == vars["VaultName"].(string) {
						found = true

						break
					}
				}
				assert.True(t, found, "vault %q should be listed", vars["VaultName"].(string))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Batch provisions Batch resources via Terraform and verifies they exist.
func TestTerraform_Batch(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "batch/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"Suffix": id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createBatchClient(t)
				suffix := vars["Suffix"].(string)

				ceOut, err := client.DescribeComputeEnvironments(ctx, &batchsvc.DescribeComputeEnvironmentsInput{
					ComputeEnvironments: []string{"tf-ce-" + suffix},
				})
				require.NoError(t, err, "DescribeComputeEnvironments should succeed")
				require.Len(t, ceOut.ComputeEnvironments, 1, "compute environment should exist")
				assert.Equal(t, "tf-ce-"+suffix, *ceOut.ComputeEnvironments[0].ComputeEnvironmentName)

				jqOut, err := client.DescribeJobQueues(ctx, &batchsvc.DescribeJobQueuesInput{
					JobQueues: []string{"tf-jq-" + suffix},
				})
				require.NoError(t, err, "DescribeJobQueues should succeed")
				require.Len(t, jqOut.JobQueues, 1, "job queue should exist")
				assert.Equal(t, "tf-jq-"+suffix, *jqOut.JobQueues[0].JobQueueName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Elasticbeanstalk provisions Elastic Beanstalk resources via Terraform and verifies they exist.
func TestTerraform_Elasticbeanstalk(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "elasticbeanstalk/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"Suffix": id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createElasticbeanstalkClient(t)
				suffix := vars["Suffix"].(string)
				appName := "tf-app-" + suffix
				envName := "tf-env-" + suffix

				appOut, err := client.DescribeApplications(ctx, &elasticbeanstalksvc.DescribeApplicationsInput{
					ApplicationNames: []string{appName},
				})
				require.NoError(t, err, "DescribeApplications should succeed")
				require.Len(t, appOut.Applications, 1, "application should exist")
				assert.Equal(t, appName, *appOut.Applications[0].ApplicationName)

				envOut, err := client.DescribeEnvironments(ctx, &elasticbeanstalksvc.DescribeEnvironmentsInput{
					EnvironmentNames: []string{envName},
				})
				require.NoError(t, err, "DescribeEnvironments should succeed")
				require.Len(t, envOut.Environments, 1, "environment should exist")
				assert.Equal(t, envName, *envOut.Environments[0].EnvironmentName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_EKS provisions an EKS cluster via Terraform, then verifies it is listed via the EKS SDK.
func TestTerraform_EKS(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "eks/cluster",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"ClusterName": "tf-eks-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createEKSClient(t)
				out, err := client.ListClusters(ctx, &ekssvc.ListClustersInput{})
				require.NoError(t, err, "ListClusters should succeed after terraform apply")
				found := slices.Contains(out.Clusters, vars["ClusterName"].(string))
				assert.True(t, found, "cluster %q should be listed", vars["ClusterName"].(string))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_EKSComprehensive provisions an EKS cluster with node group, Fargate profile,
// add-on, and access entry via Terraform, then verifies resources via the EKS SDK.
func TestTerraform_EKSComprehensive(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "eks-comprehensive/main",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"ClusterName": "tf-eks-comp-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createEKSClient(t)
				clusterName := vars["ClusterName"].(string)

				// Verify cluster exists.
				out, err := client.ListClusters(ctx, &ekssvc.ListClustersInput{})
				require.NoError(t, err, "ListClusters should succeed")
				assert.True(t, slices.Contains(out.Clusters, clusterName), "cluster %q should be listed", clusterName)

				// Verify node group exists with scaling config.
				ngOut, err := client.ListNodegroups(ctx, &ekssvc.ListNodegroupsInput{
					ClusterName: &clusterName,
				})
				require.NoError(t, err, "ListNodegroups should succeed")
				assert.True(t, slices.Contains(ngOut.Nodegroups, "workers"), "nodegroup 'workers' should be listed")

				// Verify Fargate profile exists.
				fpOut, err := client.ListFargateProfiles(ctx, &ekssvc.ListFargateProfilesInput{
					ClusterName: &clusterName,
				})
				require.NoError(t, err, "ListFargateProfiles should succeed")
				assert.True(
					t,
					slices.Contains(fpOut.FargateProfileNames, "serverless"),
					"fargate profile 'serverless' should be listed",
				)

				// Verify add-on exists.
				addonOut, err := client.ListAddons(ctx, &ekssvc.ListAddonsInput{
					ClusterName: &clusterName,
				})
				require.NoError(t, err, "ListAddons should succeed")
				assert.True(t, slices.Contains(addonOut.Addons, "coredns"), "addon 'coredns' should be listed")

				// Verify access entry exists.
				aeOut, err := client.ListAccessEntries(ctx, &ekssvc.ListAccessEntriesInput{
					ClusterName: &clusterName,
				})
				require.NoError(t, err, "ListAccessEntries should succeed")
				assert.NotEmpty(t, aeOut.AccessEntries, "access entries should not be empty")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Bedrock provisions Bedrock guardrail via Terraform and verifies it exists.
func TestTerraform_Bedrock(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "bedrock/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"Suffix": id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createBedrockClient(t)
				suffix := vars["Suffix"].(string)

				out, err := client.ListGuardrails(ctx, &bedrocksvc.ListGuardrailsInput{})
				require.NoError(t, err, "ListGuardrails should succeed")

				var found bool

				for _, g := range out.Guardrails {
					if g.Name != nil && *g.Name == "tf-guardrail-"+suffix {
						found = true

						break
					}
				}

				assert.True(t, found, "guardrail tf-guardrail-%s should exist", suffix)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_BedrockRuntime verifies that the Bedrock Runtime service is reachable
// and model invocations (InvokeModel, Converse) return the expected mock responses.
func TestTerraform_BedrockRuntime(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "bedrockruntime/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{}
			},
			verify: func(t *testing.T, ctx context.Context, _ map[string]any) {
				t.Helper()
				client := createBedrockRuntimeClient(t)

				invokeOut, err := client.InvokeModel(ctx, &bedrockruntimesvc.InvokeModelInput{
					ModelId: aws.String("anthropic.claude-v2"),
					Body:    []byte(`{"prompt":"Human: Hello\n\nAssistant:"}`),
				})
				require.NoError(t, err, "InvokeModel should succeed")
				assert.NotEmpty(t, invokeOut.Body, "InvokeModel response body should not be empty")

				converseOut, err := client.Converse(ctx, &bedrockruntimesvc.ConverseInput{
					ModelId: aws.String("anthropic.claude-3-sonnet-20240229-v1:0"),
					Messages: []bedrockruntimetypes.Message{
						{
							Role: "user",
							Content: []bedrockruntimetypes.ContentBlock{
								&bedrockruntimetypes.ContentBlockMemberText{Value: "Hello!"},
							},
						},
					},
				})
				require.NoError(t, err, "Converse should succeed")
				require.NotNil(t, converseOut.Output, "Converse output should not be nil")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Ce provisions a Cost Explorer cost category via Terraform,
// then verifies it is listed via the Ce SDK.
func TestTerraform_Ce(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "ce/cost_category",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"CategoryName": "tf-ce-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createCeClient(t)
				out, err := client.ListCostCategoryDefinitions(ctx, &cesvc.ListCostCategoryDefinitionsInput{})
				require.NoError(t, err, "ListCostCategoryDefinitions should succeed after terraform apply")

				found := false
				for _, ref := range out.CostCategoryReferences {
					if aws.ToString(ref.Name) == vars["CategoryName"].(string) {
						found = true

						break
					}
				}
				assert.True(t, found, "cost category %q should be listed", vars["CategoryName"].(string))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_CloudTrail provisions a CloudTrail trail via Terraform, then verifies
// it is reachable via the CloudTrail SDK.
func TestTerraform_CloudTrail(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "cloudtrail/trail",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"TrailName":  "tf-cloudtrail-" + id,
					"BucketName": "tf-cloudtrail-bucket-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createCloudTrailClient(t)
				trailName := vars["TrailName"].(string)
				out, err := client.DescribeTrails(ctx, &cloudtrailsvc.DescribeTrailsInput{
					TrailNameList: []string{trailName},
				})
				require.NoError(t, err, "DescribeTrails should succeed after terraform apply")
				require.NotEmpty(t, out.TrailList, "trail %q should be listed", trailName)
				assert.Equal(t, trailName, aws.ToString(out.TrailList[0].Name))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_CloudControl provisions a CloudControl API resource via Terraform,
// then verifies it is listed via the CloudControl SDK.
func TestTerraform_CloudControl(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "cloudcontrol/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"Suffix": id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createCloudControlClient(t)
				expectedName := "tf-cloudcontrol-" + vars["Suffix"].(string)

				out, err := client.ListResources(ctx, &cloudcontrolsvc.ListResourcesInput{
					TypeName: aws.String("AWS::Logs::LogGroup"),
				})
				require.NoError(t, err, "ListResources should succeed after terraform apply")

				var foundIdentifier string

				for _, rd := range out.ResourceDescriptions {
					var props map[string]any
					propsJSON := []byte(aws.ToString(rd.Properties))
					if unmarshalErr := json.Unmarshal(propsJSON, &props); unmarshalErr == nil {
						if props["LogGroupName"] == expectedName {
							foundIdentifier = aws.ToString(rd.Identifier)

							break
						}
					}

					if aws.ToString(rd.Identifier) == expectedName {
						foundIdentifier = expectedName

						break
					}
				}

				assert.NotEmpty(t, foundIdentifier, "cloudcontrol resource %q should be listed", expectedName)

				if foundIdentifier != "" {
					getOut, getErr := client.GetResource(ctx, &cloudcontrolsvc.GetResourceInput{
						TypeName:   aws.String("AWS::Logs::LogGroup"),
						Identifier: aws.String(foundIdentifier),
					})
					require.NoError(t, getErr, "GetResource should succeed after terraform apply")
					require.NotNil(t, getOut.ResourceDescription, "resource description should not be nil")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_CloudFront provisions a CloudFront OAI via Terraform and verifies it exists.
func TestTerraform_CloudFront(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "cloudfront/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{"Suffix": id}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createCloudFrontClient(t)
				suffix := vars["Suffix"].(string)
				out, err := client.ListCloudFrontOriginAccessIdentities(
					ctx,
					&cloudfrontsvc.ListCloudFrontOriginAccessIdentitiesInput{},
				)
				require.NoError(t, err, "ListCloudFrontOriginAccessIdentities should succeed after terraform apply")
				found := false
				for _, oai := range out.CloudFrontOriginAccessIdentityList.Items {
					if aws.ToString(oai.Comment) == "OAI-"+suffix {
						found = true

						break
					}
				}

				assert.True(t, found, "OAI should be listed after apply")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_CodeBuild provisions a CodeBuild project via Terraform and verifies it exists.
func TestTerraform_CodeBuild(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "codebuild/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{"Suffix": id}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createCodeBuildClient(t)
				suffix := vars["Suffix"].(string)

				out, err := client.BatchGetProjects(ctx, &codebuildsvc.BatchGetProjectsInput{
					Names: []string{"tf-project-" + suffix},
				})
				require.NoError(t, err, "BatchGetProjects should succeed")
				require.Len(t, out.Projects, 1, "project should exist")
				assert.Equal(t, "tf-project-"+suffix, aws.ToString(out.Projects[0].Name))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_CodeArtifact provisions a CodeArtifact domain and repository via Terraform
// and verifies they exist using the CodeArtifact SDK.
func TestTerraform_CodeArtifact(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "codeartifact/domain",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"DomainName":     "tf-domain-" + id,
					"RepositoryName": "tf-repo-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createCodeArtifactClient(t)
				domainName := vars["DomainName"].(string)
				repoName := vars["RepositoryName"].(string)

				// Verify the domain was created.
				domainOut, err := client.DescribeDomain(ctx, &codeartifactsvc.DescribeDomainInput{
					Domain: aws.String(domainName),
				})
				require.NoError(t, err, "DescribeDomain should succeed after terraform apply")
				require.NotNil(t, domainOut.Domain, "domain should be returned")
				assert.Equal(t, domainName, aws.ToString(domainOut.Domain.Name))

				// Verify the repository was created.
				repoOut, err := client.DescribeRepository(ctx, &codeartifactsvc.DescribeRepositoryInput{
					Domain:     aws.String(domainName),
					Repository: aws.String(repoName),
				})
				require.NoError(t, err, "DescribeRepository should succeed after terraform apply")
				require.NotNil(t, repoOut.Repository, "repository should be returned")
				assert.Equal(t, repoName, aws.ToString(repoOut.Repository.Name))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_CodeConnections provisions a CodeConnections connection via Terraform and verifies it exists.
func TestTerraform_CodeConnections(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "codeconnections/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"Suffix": id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createCodeConnectionsClient(t)
				suffix := vars["Suffix"].(string)

				out, err := client.ListConnections(ctx, &codeconnectionssvc.ListConnectionsInput{})
				require.NoError(t, err, "ListConnections should succeed")

				var found bool

				for _, conn := range out.Connections {
					if conn.ConnectionName != nil && *conn.ConnectionName == "tf-conn-"+suffix {
						found = true

						break
					}
				}

				assert.True(t, found, "connection tf-conn-%s should exist", suffix)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_CodeCommit provisions a CodeCommit repository via Terraform
// and verifies it exists using the CodeCommit SDK.
func TestTerraform_CodeCommit(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "codecommit/repository",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"RepositoryName": "tf-repo-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createCodeCommitClient(t)
				repoName := vars["RepositoryName"].(string)

				out, err := client.GetRepository(ctx, &codecommitsvc.GetRepositoryInput{
					RepositoryName: aws.String(repoName),
				})
				require.NoError(t, err, "GetRepository should succeed after terraform apply")
				require.NotNil(t, out.RepositoryMetadata, "repository metadata should be returned")
				assert.Equal(t, repoName, aws.ToString(out.RepositoryMetadata.RepositoryName))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_CodePipeline provisions a CodePipeline pipeline via Terraform and verifies it exists.
func TestTerraform_CodePipeline(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "codepipeline/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{"Suffix": id}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createCodePipelineClient(t)
				suffix := vars["Suffix"].(string)

				out, err := client.GetPipeline(ctx, &codepipelinesvc.GetPipelineInput{
					Name: aws.String("tf-pipeline-" + suffix),
				})
				require.NoError(t, err, "GetPipeline should succeed after terraform apply")
				require.NotNil(t, out.Pipeline, "pipeline should be returned")
				assert.Equal(t, "tf-pipeline-"+suffix, aws.ToString(out.Pipeline.Name))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_CodeDeploy provisions a CodeDeploy application via Terraform
// and verifies it exists using the CodeDeploy SDK.
func TestTerraform_CodeDeploy(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "codedeploy/app",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"AppName": "tf-app-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createCodeDeployClient(t)
				appName := vars["AppName"].(string)

				out, err := client.GetApplication(ctx, &codedeploysvc.GetApplicationInput{
					ApplicationName: aws.String(appName),
				})
				require.NoError(t, err, "GetApplication should succeed after terraform apply")
				require.NotNil(t, out.Application, "application should be returned")
				assert.Equal(t, appName, aws.ToString(out.Application.ApplicationName))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_DMS provisions a DMS replication instance via Terraform
// and verifies it exists using the DMS SDK.
func TestTerraform_DMS(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "dms/instance",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"InstanceID": "tf-dms-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createDMSClient(t)
				instanceID := vars["InstanceID"].(string)

				out, err := client.DescribeReplicationInstances(ctx, &dmssvc.DescribeReplicationInstancesInput{
					Filters: []dmstypes.Filter{
						{
							Name:   aws.String("replication-instance-id"),
							Values: []string{instanceID},
						},
					},
				})
				require.NoError(t, err, "DescribeReplicationInstances should succeed after terraform apply")
				require.NotEmpty(t, out.ReplicationInstances, "replication instance should be returned")
				assert.Equal(t, instanceID, aws.ToString(out.ReplicationInstances[0].ReplicationInstanceIdentifier))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_CodeStarConnections provisions a CodeStar connection via Terraform
// and verifies it exists using the CodeStar Connections SDK.
func TestTerraform_CodeStarConnections(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "codestarconnections/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{"Suffix": id}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createCodeStarConnectionsClient(t)
				suffix := vars["Suffix"].(string)

				out, err := client.ListConnections(ctx, &codestarconnectionssvc.ListConnectionsInput{})
				require.NoError(t, err)

				found := false

				for _, conn := range out.Connections {
					if aws.ToString(conn.ConnectionName) == "tf-conn-"+suffix {
						found = true
						assert.Equal(t, "GitHub", string(conn.ProviderType))

						break
					}
				}

				assert.True(t, found, "connection should exist")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_EFS provisions an EFS file system via Terraform and verifies it exists.
func TestTerraform_EFS(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "efs/filesystem",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"CreationToken": "tf-efs-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createEFSClient(t)
				token := vars["CreationToken"].(string)
				out, err := client.DescribeFileSystems(ctx, &efssvc.DescribeFileSystemsInput{})
				require.NoError(t, err, "DescribeFileSystems should succeed after terraform apply")
				found := false
				for _, fs := range out.FileSystems {
					if aws.ToString(fs.CreationToken) == token {
						found = true

						break
					}
				}
				assert.True(t, found, "file system with token %q should be listed", token)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_ELB provisions a Classic ELB load balancer via Terraform, then verifies
// it is listed via the ELB SDK.
func TestTerraform_ELB(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "elb/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"Suffix": id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createELBClient(t)
				suffix := vars["Suffix"].(string)
				name := "tf-elb-" + suffix

				out, err := client.DescribeLoadBalancers(ctx, &elbsvc.DescribeLoadBalancersInput{
					LoadBalancerNames: []string{name},
				})
				require.NoError(t, err, "DescribeLoadBalancers should succeed after terraform apply")
				require.Len(t, out.LoadBalancerDescriptions, 1, "load balancer should exist")
				assert.Equal(t, name, aws.ToString(out.LoadBalancerDescriptions[0].LoadBalancerName))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_EmrServerless provisions an EMR Serverless application via Terraform, then
// verifies it is listed via the EMR Serverless SDK.
func TestTerraform_EmrServerless(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "emrserverless/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"Suffix": id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createEmrServerlessClient(t)
				suffix := vars["Suffix"].(string)
				name := "tf-emr-" + suffix

				out, err := client.ListApplications(ctx, &emrserverlesssvc.ListApplicationsInput{})
				require.NoError(t, err, "ListApplications should succeed after terraform apply")
				found := false

				for _, app := range out.Applications {
					if aws.ToString(app.Name) == name {
						found = true

						break
					}
				}

				assert.True(t, found, "application %q should be listed", name)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_EMR provisions an EMR cluster via Terraform and verifies it exists via the EMR SDK.
func TestTerraform_EMR(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "emr/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"Suffix": id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createEMRClient(t)
				suffix := vars["Suffix"].(string)
				name := "tf-emr-" + suffix

				out, err := client.ListClusters(ctx, &emrsvc.ListClustersInput{})
				require.NoError(t, err, "ListClusters should succeed")

				found := false
				for _, c := range out.Clusters {
					if aws.ToString(c.Name) == name {
						found = true

						break
					}
				}

				assert.True(t, found, "EMR cluster %q should exist", name)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_ELBv2 provisions an ALB, target group, and listener via Terraform, then verifies
// they exist via the ELBv2 SDK.
func TestTerraform_ELBv2(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "elbv2/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"Suffix": id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createELBv2Client(t)
				suffix := vars["Suffix"].(string)
				lbName := "tf-alb-" + suffix
				tgName := "tf-tg-" + suffix

				lbOut, err := client.DescribeLoadBalancers(ctx, &elbv2svc.DescribeLoadBalancersInput{
					Names: []string{lbName},
				})
				require.NoError(t, err, "DescribeLoadBalancers should succeed after terraform apply")
				require.Len(t, lbOut.LoadBalancers, 1, "load balancer should exist")
				assert.Equal(t, lbName, *lbOut.LoadBalancers[0].LoadBalancerName)

				tgOut, err := client.DescribeTargetGroups(ctx, &elbv2svc.DescribeTargetGroupsInput{
					Names: []string{tgName},
				})
				require.NoError(t, err, "DescribeTargetGroups should succeed after terraform apply")
				require.Len(t, tgOut.TargetGroups, 1, "target group should exist")
				assert.Equal(t, tgName, *tgOut.TargetGroups[0].TargetGroupName)

				lbArn := lbOut.LoadBalancers[0].LoadBalancerArn
				listenerOut, err := client.DescribeListeners(ctx, &elbv2svc.DescribeListenersInput{
					LoadBalancerArn: lbArn,
				})
				require.NoError(t, err, "DescribeListeners should succeed after terraform apply")
				require.Len(t, listenerOut.Listeners, 1, "listener should exist")
				assert.Equal(t, *lbArn, *listenerOut.Listeners[0].LoadBalancerArn)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_FIS provisions a FIS experiment template via Terraform, then verifies
// it exists via the FIS SDK.
func TestTerraform_FIS(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "fis/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"Suffix": id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createFISClient(t)
				suffix := vars["Suffix"].(string)
				description := "tf-fis-" + suffix

				out, err := client.ListExperimentTemplates(ctx, &fissvc.ListExperimentTemplatesInput{})
				require.NoError(t, err, "ListExperimentTemplates should succeed after terraform apply")

				found := slices.ContainsFunc(
					out.ExperimentTemplates,
					func(tpl fistypes.ExperimentTemplateSummary) bool {
						return aws.ToString(tpl.Description) == description
					},
				)
				assert.True(t, found, "experiment template %q should be listed", description)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Glue provisions a Glue catalog database via Terraform, then verifies
// it exists via the Glue SDK.
func TestTerraform_Glue(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "glue/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"Suffix": id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createGlueClient(t)
				suffix := vars["Suffix"].(string)
				dbName := "tf-glue-" + suffix

				out, err := client.GetDatabase(ctx, &gluesvc.GetDatabaseInput{
					Name: aws.String(dbName),
				})
				require.NoError(t, err, "GetDatabase should succeed after terraform apply")
				require.NotNil(t, out.Database)
				assert.Equal(t, dbName, aws.ToString(out.Database.Name))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_IdentityStore provisions an identity store user, group, and membership
// via Terraform, then verifies them via the Identity Store SDK.
func TestTerraform_IdentityStore(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "identitystore/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"IdentityStoreID": "d-0000000000",
					"UserName":        "tf-user-" + id,
					"DisplayName":     "TF User " + id,
					"GivenName":       "TF",
					"FamilyName":      "User-" + id,
					"GroupName":       "tf-group-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createIdentityStoreClient(t)
				storeID := vars["IdentityStoreID"].(string)
				userName := vars["UserName"].(string)

				out, err := client.ListUsers(ctx, &identitystoresvc.ListUsersInput{
					IdentityStoreId: &storeID,
				})
				require.NoError(t, err, "ListUsers should succeed after terraform apply")

				found := slices.ContainsFunc(out.Users, func(u identitystoretypes.User) bool {
					return aws.ToString(u.UserName) == userName
				})
				assert.True(t, found, "expected user %q to exist in identity store", userName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Glacier provisions an AWS Glacier vault via Terraform, then verifies
// it is listed via the Glacier SDK.
func TestTerraform_Glacier(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "glacier/vault",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"VaultName": "tf-glacier-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createGlacierClient(t)
				vaultName := vars["VaultName"].(string)

				out, err := client.ListVaults(ctx, &glaciersvc.ListVaultsInput{
					AccountId: aws.String("-"),
				})
				require.NoError(t, err, "ListVaults should succeed after terraform apply")

				found := false
				for _, v := range out.VaultList {
					if aws.ToString(v.VaultName) == vaultName {
						found = true

						break
					}
				}

				assert.True(t, found, "vault %q should be listed", vaultName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_KinesisAnalytics provisions a Kinesis Analytics application via Terraform,
// then verifies it is listed via the Kinesis Analytics SDK.
//

func TestTerraform_KinesisAnalytics(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "kinesisanalytics/application",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"AppName": "tf-ka-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createKinesisAnalyticsClient(t)
				appName := vars["AppName"].(string)

				out, err := client.ListApplications(ctx, &kinesisanalyticssvc.ListApplicationsInput{})
				require.NoError(t, err, "ListApplications should succeed after terraform apply")

				found := false
				for _, a := range out.ApplicationSummaries {
					if aws.ToString(a.ApplicationName) == appName {
						found = true

						break
					}
				}

				assert.True(t, found, "application %q should be listed", appName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Kafka provisions an MSK cluster via Terraform, then verifies
// it is listed via the Kafka SDK.
func TestTerraform_Kafka(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "kafka/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"Suffix": id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createKafkaClient(t)
				suffix := vars["Suffix"].(string)
				clusterName := "tf-kafka-" + suffix

				out, err := client.ListClusters(ctx, &kafkasvc.ListClustersInput{
					ClusterNameFilter: aws.String(clusterName),
				})
				require.NoError(t, err, "ListClusters should succeed after terraform apply")

				found := false
				for _, cl := range out.ClusterInfoList {
					if aws.ToString(cl.ClusterName) == clusterName {
						found = true

						break
					}
				}

				assert.True(t, found, "cluster %q should be listed after terraform apply", clusterName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_KinesisAnalyticsV2 verifies that Kinesis Data Analytics v2 resources created
// via Terraform are visible through the SDK.
func TestTerraform_KinesisAnalyticsV2(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "kinesisanalyticsv2/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"Suffix": id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createKinesisAnalyticsV2Client(t)
				suffix := vars["Suffix"].(string)
				appName := "tf-kinesisanalyticsv2-" + suffix

				out, err := client.ListApplications(ctx, &kinesisanalyticsv2svc.ListApplicationsInput{})
				require.NoError(t, err, "ListApplications should succeed after terraform apply")

				found := false
				for _, app := range out.ApplicationSummaries {
					if aws.ToString(app.ApplicationName) == appName {
						found = true

						break
					}
				}

				assert.True(t, found, "application %q should be listed after terraform apply", appName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_MediaConvert provisions a MediaConvert queue via Terraform
// and verifies it exists using the MediaConvert SDK.
func TestTerraform_MediaConvert(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "mediaconvert/queue",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"QueueName": id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createMediaConvertClient(t)
				queueName := "tf-mc-queue-" + vars["QueueName"].(string)

				out, err := client.ListQueues(
					ctx,
					&mediaconvertsvc.ListQueuesInput{},
				)
				require.NoError(t, err, "ListQueues should succeed after terraform apply")

				found := false

				for _, q := range out.Queues {
					if aws.ToString(q.Name) == queueName {
						found = true

						break
					}
				}

				assert.True(t, found, "queue %q should be listed after terraform apply", queueName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_MQ provisions an Amazon MQ broker via Terraform, then verifies
// it is listed via the Amazon MQ SDK.
func TestTerraform_MQ(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "mq/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"Suffix": id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createMQClient(t)
				suffix := vars["Suffix"].(string)
				brokerName := "tf-mq-" + suffix

				out, err := client.ListBrokers(ctx, &mqsvc.ListBrokersInput{})
				require.NoError(t, err, "ListBrokers should succeed after terraform apply")

				found := false

				for _, br := range out.BrokerSummaries {
					if aws.ToString(br.BrokerName) == brokerName {
						found = true

						break
					}
				}

				assert.True(t, found, "broker %q should be listed after terraform apply", brokerName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_LakeFormation provisions Lake Formation data lake settings via Terraform,
// then verifies the settings are readable via the Lake Formation SDK.
func TestTerraform_LakeFormation(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "lakeformation/settings",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{}
			},
			verify: func(t *testing.T, ctx context.Context, _ map[string]any) {
				t.Helper()
				client := createLakeFormationClient(t)

				out, err := client.GetDataLakeSettings(ctx, &lakeformationsvc.GetDataLakeSettingsInput{})
				require.NoError(t, err, "GetDataLakeSettings should succeed after terraform apply")
				require.NotNil(t, out.DataLakeSettings, "DataLakeSettings should not be nil")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_MediaStore provisions a MediaStore container via Terraform, then verifies
// it is listed via the MediaStore SDK.
func TestTerraform_MediaStore(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "mediastore/container",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"ContainerName": "tfms_" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createMediaStoreClient(t)
				containerName := vars["ContainerName"].(string)

				out, err := client.ListContainers(ctx, &mediastoresvc.ListContainersInput{})
				require.NoError(t, err, "ListContainers should succeed after terraform apply")

				found := false
				for _, c := range out.Containers {
					if aws.ToString(c.Name) == containerName {
						found = true

						break
					}
				}

				assert.True(t, found, "container %q should be listed", containerName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_MemoryDB provisions a MemoryDB cluster via Terraform, then verifies
// it is listed via the MemoryDB SDK.
func TestTerraform_MemoryDB(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "memorydb/cluster",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"ClusterName": "tfmdb-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createMemoryDBClient(t)
				clusterName := vars["ClusterName"].(string)

				out, err := client.DescribeClusters(ctx, &memorydbsvc.DescribeClustersInput{})
				require.NoError(t, err, "DescribeClusters should succeed after terraform apply")

				found := false
				for _, c := range out.Clusters {
					if aws.ToString(c.Name) == clusterName {
						found = true

						break
					}
				}

				assert.True(t, found, "created cluster should appear in DescribeClusters output")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Organizations provisions an Organizations org and OU via Terraform,
// then verifies it via the Organizations SDK.
func TestTerraform_Organizations(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "organizations/org",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				id := uuid.NewString()[:8]

				return map[string]any{
					"Suffix": id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createOrganizationsClient(t)

				out, err := client.DescribeOrganization(ctx, &organizationssvc.DescribeOrganizationInput{})
				require.NoError(t, err, "DescribeOrganization should succeed after terraform apply")
				require.NotNil(t, out.Organization)
				assert.Equal(t, "ALL", string(out.Organization.FeatureSet))

				rootsOut, err := client.ListRoots(ctx, &organizationssvc.ListRootsInput{})
				require.NoError(t, err, "ListRoots should succeed after terraform apply")
				require.NotEmpty(t, rootsOut.Roots, "organization should have at least one root")

				suffix := vars["Suffix"].(string)
				ouName := "development-" + suffix

				ousOut, err := client.ListOrganizationalUnitsForParent(
					ctx,
					&organizationssvc.ListOrganizationalUnitsForParentInput{
						ParentId: rootsOut.Roots[0].Id,
					},
				)
				require.NoError(t, err, "ListOrganizationalUnitsForParent should succeed")

				found := false

				for _, ou := range ousOut.OrganizationalUnits {
					if aws.ToString(ou.Name) == ouName {
						found = true

						break
					}
				}

				assert.True(t, found, "created OU should appear in ListOrganizationalUnitsForParent output")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_MWAA provisions an MWAA environment via Terraform, then verifies
// it is listed via the MWAA SDK.
func TestTerraform_MWAA(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "mwaa/environment",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				id := uuid.NewString()[:8]

				return map[string]any{
					"EnvironmentName": "tfmwaa-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createMWAAClient(t)
				envName := vars["EnvironmentName"].(string)

				out, err := client.ListEnvironments(ctx, &mwaasvc.ListEnvironmentsInput{})
				require.NoError(t, err, "ListEnvironments should succeed after terraform apply")

				found := slices.Contains(out.Environments, envName)

				assert.True(t, found, "created environment should appear in ListEnvironments output")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Pinpoint provisions a Pinpoint app via Terraform, then verifies
// it is listed via the Pinpoint SDK.
func TestTerraform_Pinpoint(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "pinpoint/app",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"AppName": "tfpp-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createPinpointClient(t)
				appName := vars["AppName"].(string)

				out, err := client.GetApps(ctx, &pinpointsvc.GetAppsInput{})
				require.NoError(t, err, "GetApps should succeed after terraform apply")

				found := false

				for _, item := range out.ApplicationsResponse.Item {
					if item.Name != nil && *item.Name == appName {
						found = true

						break
					}
				}

				assert.True(t, found, "created app should appear in GetApps output")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Pipes provisions an EventBridge Pipe and verifies it is described.
func TestTerraform_Pipes(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "pipes/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"PipeName": "tf-pipe-" + id,
					"RoleName": "tf-pipe-role-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createPipesClient(t)

				out, err := client.DescribePipe(ctx, &pipessvc.DescribePipeInput{
					Name: aws.String(vars["PipeName"].(string)),
				})
				require.NoError(t, err, "DescribePipe should succeed after terraform apply")
				assert.Equal(t, vars["PipeName"].(string), aws.ToString(out.Name))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_RDSData provisions an Aurora cluster and verifies the RDS Data API is accessible.
func TestTerraform_RDSData(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:       "success",
			fixture:    "rdsdata/success",
			providerFn: rdsdataProviderBlock,
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				idSuffix := uuid.NewString()[:8]

				return map[string]any{
					"ClusterIdentifier": "tf-rdsdata-" + idSuffix,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				clusterID := vars["ClusterIdentifier"].(string)
				resourceARN := "arn:aws:rds:us-east-1:000000000000:cluster:" + clusterID
				secretARN := "arn:aws:secretsmanager:us-east-1:000000000000:secret:rdsdata-test"

				client := createRDSDataClient(t)

				out, err := client.ExecuteStatement(ctx, &rdsdatasvc.ExecuteStatementInput{
					ResourceArn: aws.String(resourceARN),
					SecretArn:   aws.String(secretARN),
					Sql:         aws.String("SELECT 1"),
				})
				require.NoError(t, err, "ExecuteStatement should succeed after terraform apply")
				assert.NotNil(t, out)
			},
		},
		{
			// engine_roundtrip provisions an Aurora cluster, then drives the
			// full Data API data path against it: create a table, insert rows,
			// and read them back asserting the real returned values. The
			// terraform harness destroys the cluster on cleanup, so this
			// exercises create-engine -> add-data -> query -> destroy.
			name:       "engine_roundtrip",
			fixture:    "rdsdata/success",
			providerFn: rdsdataProviderBlock,
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{
					"ClusterIdentifier": "tf-rdsdata-rt-" + uuid.NewString()[:8],
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				clusterID := vars["ClusterIdentifier"].(string)
				resourceARN := "arn:aws:rds:us-east-1:000000000000:cluster:" + clusterID
				secretARN := "arn:aws:secretsmanager:us-east-1:000000000000:secret:rdsdata-rt"

				client := createRDSDataClient(t)

				exec := func(sql string, includeMeta bool) *rdsdatasvc.ExecuteStatementOutput {
					out, err := client.ExecuteStatement(ctx, &rdsdatasvc.ExecuteStatementInput{
						ResourceArn:           aws.String(resourceARN),
						SecretArn:             aws.String(secretARN),
						Sql:                   aws.String(sql),
						IncludeResultMetadata: includeMeta,
					})
					require.NoError(t, err, "ExecuteStatement(%q) should succeed", sql)

					return out
				}

				// Create the engine schema and add data.
				exec("CREATE TABLE tf_items (id INTEGER, name TEXT)", false)
				exec("INSERT INTO tf_items (id, name) VALUES (1, 'gopher')", false)
				exec("INSERT INTO tf_items (id, name) VALUES (2, 'stack')", false)

				// Query it back and assert the real round-tripped values.
				out := exec("SELECT id, name FROM tf_items ORDER BY id", true)
				require.Len(t, out.Records, 2, "both inserted rows should be returned")
				require.Len(t, out.ColumnMetadata, 2, "two columns should be described")

				id1, ok := out.Records[0][0].(*rdsdatatypes.FieldMemberLongValue)
				require.True(t, ok, "id column should be a long value")
				assert.Equal(t, int64(1), id1.Value)

				name1, ok := out.Records[0][1].(*rdsdatatypes.FieldMemberStringValue)
				require.True(t, ok, "name column should be a string value")
				assert.Equal(t, "gopher", name1.Value)

				name2, ok := out.Records[1][1].(*rdsdatatypes.FieldMemberStringValue)
				require.True(t, ok, "name column should be a string value")
				assert.Equal(t, "stack", name2.Value)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_RAM provisions an AWS RAM resource share and verifies it is described.
func TestTerraform_RAM(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "ram/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"ShareName": "tf-ram-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createRAMClient(t)

				out, err := client.GetResourceShares(ctx, &ramsvc.GetResourceSharesInput{
					ResourceOwner: ramsvc_types.ResourceOwnerSelf,
					Name:          aws.String(vars["ShareName"].(string)),
				})
				require.NoError(t, err, "GetResourceShares should succeed after terraform apply")
				require.NotEmpty(t, out.ResourceShares, "expected at least one resource share")
				assert.Equal(t, vars["ShareName"].(string), aws.ToString(out.ResourceShares[0].Name))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_RedshiftData provisions a Redshift Data SQL statement and verifies it was created.
func TestTerraform_RedshiftData(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "redshiftdata/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"ClusterID": "tf-cluster-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, _ map[string]any) {
				t.Helper()
				client := createRedshiftDataClient(t)

				out, err := client.ListStatements(ctx, &redshiftdatasvc.ListStatementsInput{})
				require.NoError(t, err, "ListStatements should succeed after terraform apply")
				require.NotEmpty(t, out.Statements, "expected at least one statement after apply")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_SageMaker provisions a SageMaker model and verifies it is described.
func TestTerraform_SageMaker(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "sagemaker/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"ModelName": "tf-sm-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()
				client := createSageMakerClient(t)

				out, err := client.DescribeModel(ctx, &sagemakersvc.DescribeModelInput{
					ModelName: aws.String(vars["ModelName"].(string)),
				})
				require.NoError(t, err, "DescribeModel should succeed after terraform apply")
				require.NotNil(t, out)
				assert.Equal(t, vars["ModelName"].(string), aws.ToString(out.ModelName))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_ServiceDiscovery provisions a Cloud Map namespace and service via Terraform,
// then verifies they are listed.
func TestTerraform_ServiceDiscovery(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "servicediscovery/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"NamespaceName": "tf-ns-" + id,
					"ServiceName":   "tf-svc-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, _ map[string]any) {
				t.Helper()
				client := createServiceDiscoveryClient(t)

				out, err := client.ListNamespaces(ctx, &servicediscoverysvc.ListNamespacesInput{})
				require.NoError(t, err, "ListNamespaces should succeed after terraform apply")
				require.NotEmpty(t, out.Namespaces, "expected at least one namespace after apply")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_SageMakerRuntime verifies that the SageMaker Runtime service is reachable
// and endpoint invocations succeed via the SDK directly (no Terraform-managed resources).
func TestTerraform_SageMakerRuntime(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "sagemakerruntime/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{}
			},
			verify: func(t *testing.T, ctx context.Context, _ map[string]any) {
				t.Helper()
				client := createSageMakerRuntimeClient(t)

				out, err := client.InvokeEndpoint(ctx, &sagemakerruntimesvc.InvokeEndpointInput{
					EndpointName: aws.String("tf-test-endpoint"),
					Body:         []byte(`{"data": "test input"}`),
					ContentType:  aws.String("application/json"),
				})
				require.NoError(t, err, "InvokeEndpoint should succeed")
				assert.NotEmpty(t, out.Body, "InvokeEndpoint response body should not be empty")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_ServerlessRepo provisions a Serverless Application Repository application and verifies it was created.
func TestTerraform_ServerlessRepo(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "serverlessrepo/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"ApplicationName": "tf-sar-" + id,
					"Endpoint":        endpoint,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				appName := vars["ApplicationName"].(string)
				reqURL := endpoint + "/applications/" + appName

				req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
				require.NoError(t, err)

				// Use a fake SigV4 credential scoped to the "serverlessrepo" service.
				const fakeAuth = "AWS4-HMAC-SHA256 Credential=test/20240101/us-east-1/" +
					"serverlessrepo/aws4_request, SignedHeaders=host, Signature=fake"
				req.Header.Set("Authorization", fakeAuth)

				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err, "GetApplication should succeed after terraform apply")
				defer resp.Body.Close()

				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				assert.Equal(t, http.StatusOK, resp.StatusCode, "expected 200 OK, body: %s", string(body))

				var result map[string]any
				require.NoError(t, json.Unmarshal(body, &result))
				assert.Equal(t, appName, result["name"])
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Shield provisions a Shield subscription and protection and verifies the subscription state.
func TestTerraform_Shield(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "shield/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"ProtectionName": "tf-shield-" + id,
					"ResourceARN":    "arn:aws:ec2:us-east-1:123456789012:eip-allocation/eipalloc-" + id,
					"Endpoint":       endpoint,
				}
			},
			verify: func(t *testing.T, ctx context.Context, _ map[string]any) {
				t.Helper()

				reqURL := endpoint + "/"
				body := `{}`

				req, err := http.NewRequestWithContext(
					ctx, http.MethodPost, reqURL, strings.NewReader(body),
				)
				require.NoError(t, err)

				req.Header.Set("Content-Type", "application/x-amz-json-1.1")
				req.Header.Set("X-Amz-Target", "AWSShield_20160616.GetSubscriptionState")

				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err, "GetSubscriptionState should succeed after terraform apply")
				defer resp.Body.Close()

				respBody, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				assert.Equal(t, http.StatusOK, resp.StatusCode, "expected 200 OK, body: %s", string(respBody))

				var result map[string]string
				require.NoError(t, json.Unmarshal(respBody, &result))
				assert.Equal(t, "ACTIVE", result["SubscriptionState"])
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_SsoAdmin provisions an SSO Admin permission set via Terraform and verifies it was created.
func TestTerraform_SsoAdmin(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "ssoadmin/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"PermissionSetName": "tf-ps-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createSsoAdminClient(t)
				psName := vars["PermissionSetName"].(string)

				instancesOut, err := client.ListInstances(ctx, &ssoadminsvc.ListInstancesInput{})
				require.NoError(t, err, "ListInstances should succeed after terraform apply")
				require.NotEmpty(t, instancesOut.Instances, "expected at least one SSO instance")

				instanceArn := aws.ToString(instancesOut.Instances[0].InstanceArn)

				out, err := client.ListPermissionSets(ctx, &ssoadminsvc.ListPermissionSetsInput{
					InstanceArn: &instanceArn,
				})
				require.NoError(t, err, "ListPermissionSets should succeed")

				found := false

				for _, psArn := range out.PermissionSets {
					desc, descErr := client.DescribePermissionSet(ctx, &ssoadminsvc.DescribePermissionSetInput{
						InstanceArn:      &instanceArn,
						PermissionSetArn: &psArn,
					})
					if descErr == nil && aws.ToString(desc.PermissionSet.Name) == psName {
						found = true

						break
					}
				}

				assert.True(t, found, "expected permission set %q to exist", psName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Textract provisions a Textract text detection job and verifies the service responds.
func TestTerraform_Textract(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "textract/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{
					"Endpoint": endpoint,
					"Bucket":   "tf-textract-bucket",
					"Key":      "document-" + uuid.NewString()[:8] + ".pdf",
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				bucket, _ := vars["Bucket"].(string)
				key, _ := vars["Key"].(string)

				reqURL := endpoint + "/"
				body := fmt.Sprintf(`{"Document":{"S3Object":{"Bucket":"%s","Name":"%s"}}}`, bucket, key)

				req, err := http.NewRequestWithContext(
					ctx, http.MethodPost, reqURL, strings.NewReader(body),
				)
				require.NoError(t, err)

				req.Header.Set("Content-Type", "application/x-amz-json-1.1")
				req.Header.Set("X-Amz-Target", "Textract.DetectDocumentText")

				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err, "DetectDocumentText should succeed")
				defer resp.Body.Close()

				respBody, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				assert.Equal(t, http.StatusOK, resp.StatusCode, "expected 200 OK, body: %s", string(respBody))

				var result map[string]any
				require.NoError(t, json.Unmarshal(respBody, &result))
				blocks, ok := result["Blocks"].([]any)
				assert.True(t, ok, "expected Blocks in response")
				assert.NotEmpty(t, blocks, "expected non-empty Blocks")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_TimestreamWrite provisions a Timestream Write database and table, then verifies.
func TestTerraform_TimestreamWrite(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "timestreamwrite/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				id := uuid.NewString()[:8]

				return map[string]any{
					"Endpoint":     endpoint,
					"DatabaseName": "tf-tswrite-db-" + id,
					"TableName":    "tf-tswrite-tbl-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				databaseName, ok := vars["DatabaseName"].(string)
				require.True(t, ok, "DatabaseName must be a string")

				reqURL := endpoint + "/"
				body := fmt.Sprintf(`{"DatabaseName":"%s"}`, databaseName)

				req, err := http.NewRequestWithContext(
					ctx, http.MethodPost, reqURL, strings.NewReader(body),
				)
				require.NoError(t, err)

				req.Header.Set("Content-Type", "application/x-amz-json-1.1")
				req.Header.Set("X-Amz-Target", "Timestream_20181101.DescribeDatabase")

				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err, "DescribeDatabase should succeed")
				defer resp.Body.Close()

				respBody, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				assert.Equal(t, http.StatusOK, resp.StatusCode, "expected 200 OK, body: %s", string(respBody))

				var result map[string]any
				require.NoError(t, json.Unmarshal(respBody, &result))
				db, ok := result["Database"].(map[string]any)
				assert.True(t, ok, "expected Database in response")
				assert.Equal(t, databaseName, db["DatabaseName"])
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_TimestreamQuery provisions Timestream Query resources via Terraform and verifies operations.
func TestTerraform_TimestreamQuery(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "timestreamquery/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{
					"Endpoint": endpoint,
				}
			},
			verify: func(t *testing.T, ctx context.Context, _ map[string]any) {
				t.Helper()

				client := createTimestreamQueryClient(t)

				// Verify ListScheduledQueries returns an empty list.
				listOut, err := client.ListScheduledQueries(ctx, &timestreamquerysvc.ListScheduledQueriesInput{})
				require.NoError(t, err, "ListScheduledQueries should succeed")
				require.NotNil(t, listOut)

				// Create a scheduled query.
				createOut, err := client.CreateScheduledQuery(ctx, &timestreamquerysvc.CreateScheduledQueryInput{
					Name:        aws.String("tf-tsq-verify"),
					QueryString: aws.String("SELECT 1"),
					ScheduleConfiguration: &timestreamquerytypes.ScheduleConfiguration{
						ScheduleExpression: aws.String("rate(1 hour)"),
					},
					ScheduledQueryExecutionRoleArn: aws.String("arn:aws:iam::000000000000:role/verify"),
					NotificationConfiguration: &timestreamquerytypes.NotificationConfiguration{
						SnsConfiguration: &timestreamquerytypes.SnsConfiguration{
							TopicArn: aws.String("arn:aws:sns:us-east-1:000000000000:verify"),
						},
					},
					ErrorReportConfiguration: &timestreamquerytypes.ErrorReportConfiguration{
						S3Configuration: &timestreamquerytypes.S3Configuration{
							BucketName: aws.String("verify-bucket"),
						},
					},
				})
				require.NoError(t, err, "CreateScheduledQuery should succeed")
				require.NotNil(t, createOut.Arn)

				arn := aws.ToString(createOut.Arn)

				// Describe the created scheduled query.
				descOut, err := client.DescribeScheduledQuery(ctx, &timestreamquerysvc.DescribeScheduledQueryInput{
					ScheduledQueryArn: aws.String(arn),
				})
				require.NoError(t, err, "DescribeScheduledQuery should succeed")
				assert.Equal(t, "tf-tsq-verify", aws.ToString(descOut.ScheduledQuery.Name))

				// Delete the scheduled query.
				_, err = client.DeleteScheduledQuery(ctx, &timestreamquerysvc.DeleteScheduledQueryInput{
					ScheduledQueryArn: aws.String(arn),
				})
				require.NoError(t, err, "DeleteScheduledQuery should succeed")

				// Verify the list is empty again.
				listAfter, err := client.ListScheduledQueries(ctx, &timestreamquerysvc.ListScheduledQueriesInput{})
				require.NoError(t, err, "ListScheduledQueries after delete should succeed")
				assert.Empty(t, listAfter.ScheduledQueries)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_Transfer provisions a Transfer server via Terraform and verifies the service responds.
func TestTerraform_Transfer(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "transfer/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{
					"Endpoint": endpoint,
				}
			},
			verify: func(t *testing.T, ctx context.Context, _ map[string]any) {
				t.Helper()

				client := createTransferClient(t)

				// List servers - should have at least one from Terraform provisioning.
				listOut, err := client.ListServers(ctx, &transfersvc.ListServersInput{})
				require.NoError(t, err, "ListServers should succeed")
				assert.NotEmpty(t, listOut.Servers, "expected at least one server")

				serverID := *listOut.Servers[0].ServerId

				// Describe the server.
				descOut, err := client.DescribeServer(ctx, &transfersvc.DescribeServerInput{
					ServerId: &serverID,
				})
				require.NoError(t, err, "DescribeServer should succeed")
				assert.Equal(t, serverID, *descOut.Server.ServerId)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_VerifiedPermissions provisions a Verified Permissions policy store
// via Terraform and verifies the service responds.
func TestTerraform_VerifiedPermissions(t *testing.T) {
	t.Parallel()

	runTfTestWithEndpoint(t, "verifiedpermissions/success", func(t *testing.T, ctx context.Context) {
		t.Helper()

		client := createVerifiedPermissionsClient(t)

		// List policy stores - should have at least one from Terraform provisioning.
		listOut, err := client.ListPolicyStores(ctx, &verifiedpermissionssvc.ListPolicyStoresInput{})
		require.NoError(t, err, "ListPolicyStores should succeed")
		assert.NotEmpty(t, listOut.PolicyStores, "expected at least one policy store")

		policyStoreID := *listOut.PolicyStores[0].PolicyStoreId

		// Get the policy store.
		getOut, err := client.GetPolicyStore(ctx, &verifiedpermissionssvc.GetPolicyStoreInput{
			PolicyStoreId: &policyStoreID,
		})
		require.NoError(t, err, "GetPolicyStore should succeed")
		assert.Equal(t, policyStoreID, *getOut.PolicyStoreId)
	})
}

// TestTerraform_Xray provisions an X-Ray group via Terraform and verifies the service responds.
func TestTerraform_Xray(t *testing.T) {
	t.Parallel()

	runTfTestWithEndpoint(t, "xray/success", func(t *testing.T, ctx context.Context) {
		t.Helper()

		client := createXrayClient(t)

		// GetGroups - should have at least one from Terraform provisioning.
		groupsOut, err := client.GetGroups(ctx, &xraysvc.GetGroupsInput{})
		require.NoError(t, err, "GetGroups should succeed")
		assert.NotEmpty(t, groupsOut.Groups, "expected at least one group")
	})
}

// TestTerraform_VPCLattice provisions a VPC Lattice service and service network
// via Terraform and verifies they were created.
func TestTerraform_VPCLattice(t *testing.T) {
	t.Parallel()

	runTfTestWithEndpoint(t, "vpclattice/success", func(t *testing.T, ctx context.Context) {
		t.Helper()

		client := createVPCLatticeClient(t)

		// ListServices - should have at least one from Terraform provisioning.
		svcsOut, err := client.ListServices(ctx, &vpclatticesvc.ListServicesInput{})
		require.NoError(t, err, "ListServices should succeed")
		assert.NotEmpty(t, svcsOut.Items, "expected at least one service")

		// ListServiceNetworks - should have at least one from Terraform provisioning.
		snsOut, err := client.ListServiceNetworks(ctx, &vpclatticesvc.ListServiceNetworksInput{})
		require.NoError(t, err, "ListServiceNetworks should succeed")
		assert.NotEmpty(t, snsOut.Items, "expected at least one service network")
	})
}

// TestTerraform_Wafv2 provisions a WAFv2 Web ACL via Terraform and verifies it was created.
func TestTerraform_Wafv2(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "wafv2/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"WebACLName": "tf-wafv2-" + id,
					"Endpoint":   endpoint,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				reqURL := endpoint + "/"
				body := `{"Scope":"REGIONAL"}`

				req, err := http.NewRequestWithContext(
					ctx, http.MethodPost, reqURL, strings.NewReader(body),
				)
				require.NoError(t, err)

				req.Header.Set("Content-Type", "application/x-amz-json-1.1")
				req.Header.Set("X-Amz-Target", "AWSWAF_20190729.ListWebACLs")

				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err, "ListWebACLs should succeed after terraform apply")
				defer resp.Body.Close()

				respBody, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				assert.Equal(t, http.StatusOK, resp.StatusCode, "expected 200 OK, body: %s", string(respBody))

				var result map[string]any
				require.NoError(t, json.Unmarshal(respBody, &result))

				webACLs, ok := result["WebACLs"].([]any)
				require.True(t, ok)
				assert.NotEmpty(t, webACLs, "expected at least one Web ACL")

				found := false

				for _, item := range webACLs {
					acl, isACL := item.(map[string]any)
					if !isACL {
						continue
					}

					if acl["Name"] == vars["WebACLName"] {
						found = true

						break
					}
				}

				assert.True(t, found, "expected to find web ACL %q in list", vars["WebACLName"])
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_S3Tables provisions S3 Tables resources via Terraform and verifies the service responds.
func TestTerraform_S3Tables(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "s3tables/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				id := uuid.NewString()[:8]

				return map[string]any{
					"Suffix":   id,
					"Endpoint": endpoint,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createS3TablesClient(t)

				suffix, ok := vars["Suffix"].(string)
				require.True(t, ok, "Suffix must be a string")

				bucketName := "tf-s3t-" + suffix
				tableName := "tftable" + suffix
				nsName := "tfns" + suffix

				// ListTableBuckets should include our bucket.
				listOut, err := client.ListTableBuckets(ctx, &s3tablessvc.ListTableBucketsInput{})
				require.NoError(t, err, "ListTableBuckets should succeed")

				var bucketARN string

				for _, b := range listOut.TableBuckets {
					if aws.ToString(b.Name) == bucketName {
						bucketARN = aws.ToString(b.Arn)

						break
					}
				}

				require.NotEmpty(t, bucketARN, "expected to find table bucket %q in list", bucketName)

				// GetTableBucket should return the bucket.
				getOut, err := client.GetTableBucket(ctx, &s3tablessvc.GetTableBucketInput{
					TableBucketARN: aws.String(bucketARN),
				})
				require.NoError(t, err)
				assert.Equal(t, bucketName, aws.ToString(getOut.Name))

				// GetTable should return the table created by TF.
				getTblOut, err := client.GetTable(ctx, &s3tablessvc.GetTableInput{
					TableBucketARN: aws.String(bucketARN),
					Namespace:      aws.String(nsName),
					Name:           aws.String(tableName),
				})
				require.NoError(t, err)
				assert.Equal(t, tableName, aws.ToString(getTblOut.Name))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_ECR provisions an ECR repository via Terraform and verifies it exists.
func TestTerraform_ECR(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "ecr/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"RepoName": "tf-ecr-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createECRClient(t)
				out, err := client.DescribeRepositories(ctx, &ecrsvc.DescribeRepositoriesInput{
					RepositoryNames: []string{vars["RepoName"].(string)},
				})
				require.NoError(t, err, "DescribeRepositories should succeed after terraform apply")
				require.Len(t, out.Repositories, 1)
				assert.Equal(t, vars["RepoName"].(string), aws.ToString(out.Repositories[0].RepositoryName))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_SQS provisions SQS queues via Terraform and verifies they exist.
func TestTerraform_SQS(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "sqs/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"QueueName":     "tf-sqs-" + id,
					"FIFOQueueName": "tf-sqs-fifo-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createSQSClient(t)

				// Verify standard queue exists.
				urlOut, err := client.GetQueueUrl(ctx, &sqssvc.GetQueueUrlInput{
					QueueName: aws.String(vars["QueueName"].(string)),
				})
				require.NoError(t, err, "GetQueueUrl should succeed for standard queue")
				assert.Contains(t, aws.ToString(urlOut.QueueUrl), vars["QueueName"].(string))

				// Verify FIFO queue exists.
				fifoName := vars["FIFOQueueName"].(string) + ".fifo"
				fifoOut, err := client.GetQueueUrl(ctx, &sqssvc.GetQueueUrlInput{
					QueueName: aws.String(fifoName),
				})
				require.NoError(t, err, "GetQueueUrl should succeed for FIFO queue")
				assert.Contains(t, aws.ToString(fifoOut.QueueUrl), fifoName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_SNS provisions an SNS topic via Terraform and verifies it exists.
func TestTerraform_SNS(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "sns/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"TopicName": "tf-sns-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createSNSClient(t)
				out, err := client.ListTopics(ctx, &snssvc.ListTopicsInput{})
				require.NoError(t, err, "ListTopics should succeed after terraform apply")

				topicName := vars["TopicName"].(string)
				found := false

				for _, topic := range out.Topics {
					if strings.Contains(aws.ToString(topic.TopicArn), topicName) {
						found = true

						break
					}
				}

				assert.True(t, found, "SNS topic %q should be listed", topicName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_S3 provisions an S3 bucket with versioning via Terraform and verifies it exists.
func TestTerraform_S3(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "s3/success",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()

				return map[string]any{
					"BucketName": "tf-s3-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				client := createS3Client(t)
				_, err := client.HeadBucket(ctx, &s3svc.HeadBucketInput{
					Bucket: aws.String(vars["BucketName"].(string)),
				})
				require.NoError(t, err, "HeadBucket should succeed after terraform apply")

				// Verify versioning is enabled.
				vOut, err := client.GetBucketVersioning(ctx, &s3svc.GetBucketVersioningInput{
					Bucket: aws.String(vars["BucketName"].(string)),
				})
				require.NoError(t, err, "GetBucketVersioning should succeed")
				assert.Equal(t, s3types.BucketVersioningStatusEnabled, vOut.Status)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_CachingMessagingComprehensive provisions ElastiCache, MemoryDB, and MSK resources
// together and verifies all services are reachable and configured correctly.
func TestTerraform_CachingMessagingComprehensive(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "caching-messaging-comprehensive/main",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()
				id := uuid.NewString()[:8]

				return map[string]any{
					"Prefix": "tf-cmc-" + id,
				}
			},
			verify: func(t *testing.T, ctx context.Context, vars map[string]any) {
				t.Helper()

				prefix := vars["Prefix"].(string)

				// Verify ElastiCache replication group was created.
				ecClient := createElastiCacheClient(t)
				rgID := prefix + "-rg"
				rgOut, err := ecClient.DescribeReplicationGroups(ctx, &elasticachesvc.DescribeReplicationGroupsInput{
					ReplicationGroupId: aws.String(rgID),
				})
				require.NoError(t, err, "DescribeReplicationGroups should succeed")
				require.Len(t, rgOut.ReplicationGroups, 1)
				assert.Equal(t, rgID, aws.ToString(rgOut.ReplicationGroups[0].ReplicationGroupId))

				// Create and verify an ElastiCache snapshot via the AWS SDK, exercising
				// the CreateSnapshot and DescribeSnapshots service methods directly.
				// aws_elasticache_snapshot is not a resource type in the hashicorp/aws
				// provider, so we create the snapshot here using the SDK.
				snapName := prefix + "-snap"
				_, err = ecClient.CreateSnapshot(ctx, &elasticachesvc.CreateSnapshotInput{
					SnapshotName:       aws.String(snapName),
					ReplicationGroupId: aws.String(rgID),
				})
				require.NoError(t, err, "CreateSnapshot should succeed")

				snapOut, err := ecClient.DescribeSnapshots(ctx, &elasticachesvc.DescribeSnapshotsInput{
					SnapshotName: aws.String(snapName),
				})
				require.NoError(t, err, "DescribeSnapshots should succeed")
				require.Len(t, snapOut.Snapshots, 1)
				assert.Equal(t, snapName, aws.ToString(snapOut.Snapshots[0].SnapshotName))

				// Verify MemoryDB cluster was created.
				mdbClient := createMemoryDBClient(t)
				mdbName := prefix + "-mdb"
				mdbOut, err := mdbClient.DescribeClusters(ctx, &memorydbsvc.DescribeClustersInput{})
				require.NoError(t, err, "DescribeClusters should succeed")
				foundMDB := false
				for _, c := range mdbOut.Clusters {
					if aws.ToString(c.Name) == mdbName {
						foundMDB = true

						break
					}
				}
				assert.True(t, foundMDB, "MemoryDB cluster %q should exist", mdbName)

				// Verify MSK cluster was created with SASL/IAM auth.
				kafkaClient := createKafkaClient(t)
				kafkaName := prefix + "-kafka"
				kafkaOut, err := kafkaClient.ListClusters(ctx, &kafkasvc.ListClustersInput{
					ClusterNameFilter: aws.String(kafkaName),
				})
				require.NoError(t, err, "ListClusters should succeed")
				foundKafka := false
				for _, cl := range kafkaOut.ClusterInfoList {
					if aws.ToString(cl.ClusterName) == kafkaName {
						foundKafka = true
						require.NotNil(t, cl.ClientAuthentication, "ClientAuthentication should be set")
						require.NotNil(t, cl.ClientAuthentication.Sasl, "SASL should be set")
						require.NotNil(t, cl.ClientAuthentication.Sasl.Iam, "IAM SASL should be set")
						assert.True(
							t,
							aws.ToBool(cl.ClientAuthentication.Sasl.Iam.Enabled),
							"IAM SASL should be enabled",
						)

						break
					}
				}
				assert.True(t, foundKafka, "MSK cluster %q should exist", kafkaName)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}

// TestTerraform_MegaBatch4 provisions Bedrock Agent resources and verifies they exist.
func TestTerraform_MegaBatch4(t *testing.T) {
	t.Parallel()

	tests := []tfTestCase{
		{
			name:    "success",
			fixture: "mega-batch-4",
			setup: func(t *testing.T, _ string) map[string]any {
				t.Helper()

				return map[string]any{}
			},
			verify: func(t *testing.T, ctx context.Context, _ map[string]any) {
				t.Helper()
				client := createBedrockAgentClient(t)

				agentsOut, err := client.ListAgents(ctx, &bedrockagentsvc.ListAgentsInput{})
				require.NoError(t, err, "ListAgents should succeed")
				require.NotEmpty(t, agentsOut.AgentSummaries, "at least one agent should exist")

				kbOut, err := client.ListKnowledgeBases(ctx, &bedrockagentsvc.ListKnowledgeBasesInput{})
				require.NoError(t, err, "ListKnowledgeBases should succeed")
				require.NotEmpty(t, kbOut.KnowledgeBaseSummaries, "at least one knowledge base should exist")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runTFTest(t, tc)
		})
	}
}
