package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"net"
	"net/http"
	nhpprof "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	amplifysdk "github.com/aws/aws-sdk-go-v2/service/amplify"
	appsyncsdksvc "github.com/aws/aws-sdk-go-v2/service/appsync"
	codedeploysdk "github.com/aws/aws-sdk-go-v2/service/codedeploy"
	codepipelinesdk "github.com/aws/aws-sdk-go-v2/service/codepipeline"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbsdktypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	awsddbstreams "github.com/aws/aws-sdk-go-v2/service/dynamodbstreams"
	ddbstreamstypes "github.com/aws/aws-sdk-go-v2/service/dynamodbstreams/types"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ekssdk "github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iotsdk "github.com/aws/aws-sdk-go-v2/service/iot"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	sqssdk "github.com/aws/aws-sdk-go-v2/service/sqs"
	ssmsdk "github.com/aws/aws-sdk-go-v2/service/ssm"
	stssdk "github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/labstack/echo/v5"

	"github.com/blackbirdworks/gopherstack/dashboard"
	"github.com/blackbirdworks/gopherstack/demo"
	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/awsmeta"
	"github.com/blackbirdworks/gopherstack/pkgs/chaos"
	"github.com/blackbirdworks/gopherstack/pkgs/config"
	gopherDNS "github.com/blackbirdworks/gopherstack/pkgs/dns"
	snsevents "github.com/blackbirdworks/gopherstack/pkgs/events"
	"github.com/blackbirdworks/gopherstack/pkgs/httputils"
	"github.com/blackbirdworks/gopherstack/pkgs/inithooks"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/portalloc"
	"github.com/blackbirdworks/gopherstack/pkgs/service"
	svctags "github.com/blackbirdworks/gopherstack/pkgs/tags"
	"github.com/blackbirdworks/gopherstack/pkgs/telemetry"
	"github.com/blackbirdworks/gopherstack/pkgs/version"
	accessanalyzerbackend "github.com/blackbirdworks/gopherstack/services/accessanalyzer"
	accountbackend "github.com/blackbirdworks/gopherstack/services/account"
	acmbackend "github.com/blackbirdworks/gopherstack/services/acm"
	acmpcabackend "github.com/blackbirdworks/gopherstack/services/acmpca"
	amplifybackend "github.com/blackbirdworks/gopherstack/services/amplify"
	apigwbackend "github.com/blackbirdworks/gopherstack/services/apigateway"
	apigwmgmtbackend "github.com/blackbirdworks/gopherstack/services/apigatewaymanagementapi"
	apigwv2backend "github.com/blackbirdworks/gopherstack/services/apigatewayv2"
	appconfigbackend "github.com/blackbirdworks/gopherstack/services/appconfig"
	appconfigdatabackend "github.com/blackbirdworks/gopherstack/services/appconfigdata"
	applicationautoscalingbackend "github.com/blackbirdworks/gopherstack/services/applicationautoscaling"
	appmeshbackend "github.com/blackbirdworks/gopherstack/services/appmesh"
	apprunnerbackend "github.com/blackbirdworks/gopherstack/services/apprunner"
	appstreambackend "github.com/blackbirdworks/gopherstack/services/appstream"
	appsyncbackend "github.com/blackbirdworks/gopherstack/services/appsync"
	athenabackend "github.com/blackbirdworks/gopherstack/services/athena"
	autoscalingbackend "github.com/blackbirdworks/gopherstack/services/autoscaling"
	awsconfigbackend "github.com/blackbirdworks/gopherstack/services/awsconfig"
	azureblobbackend "github.com/blackbirdworks/gopherstack/services/azureblob"
	azurequeuebackend "github.com/blackbirdworks/gopherstack/services/azurequeue"
	azureservicebusbackend "github.com/blackbirdworks/gopherstack/services/azureservicebus"
	azuretablebackend "github.com/blackbirdworks/gopherstack/services/azuretable"
	backupbackend "github.com/blackbirdworks/gopherstack/services/backup"
	batchbackend "github.com/blackbirdworks/gopherstack/services/batch"
	bedrockbackend "github.com/blackbirdworks/gopherstack/services/bedrock"
	bedrockagentbackend "github.com/blackbirdworks/gopherstack/services/bedrockagent"
	bedrockruntimebackend "github.com/blackbirdworks/gopherstack/services/bedrockruntime"
	cebackend "github.com/blackbirdworks/gopherstack/services/ce"
	cleanroomsbackend "github.com/blackbirdworks/gopherstack/services/cleanrooms"
	cloudcontrolbackend "github.com/blackbirdworks/gopherstack/services/cloudcontrol"
	cfnbackend "github.com/blackbirdworks/gopherstack/services/cloudformation"
	cloudfrontbackend "github.com/blackbirdworks/gopherstack/services/cloudfront"
	cfkvsbackend "github.com/blackbirdworks/gopherstack/services/cloudfrontkeyvaluestore"
	cloudtrailbackend "github.com/blackbirdworks/gopherstack/services/cloudtrail"
	cwbackend "github.com/blackbirdworks/gopherstack/services/cloudwatch"
	cwlogsbackend "github.com/blackbirdworks/gopherstack/services/cloudwatchlogs"
	codeartifactbackend "github.com/blackbirdworks/gopherstack/services/codeartifact"
	codebuildbackend "github.com/blackbirdworks/gopherstack/services/codebuild"
	codecommitbackend "github.com/blackbirdworks/gopherstack/services/codecommit"
	codeconnectionsbackend "github.com/blackbirdworks/gopherstack/services/codeconnections"
	codedeploybackend "github.com/blackbirdworks/gopherstack/services/codedeploy"
	codepipelinebackend "github.com/blackbirdworks/gopherstack/services/codepipeline"
	codestarconnectionsbackend "github.com/blackbirdworks/gopherstack/services/codestarconnections"
	cognitoidentitybackend "github.com/blackbirdworks/gopherstack/services/cognitoidentity"
	cognitoidpbackend "github.com/blackbirdworks/gopherstack/services/cognitoidp"
	comprehendbackend "github.com/blackbirdworks/gopherstack/services/comprehend"
	cosmosdbbackend "github.com/blackbirdworks/gopherstack/services/cosmosdb"
	databrewbackend "github.com/blackbirdworks/gopherstack/services/databrew"
	datasyncbackend "github.com/blackbirdworks/gopherstack/services/datasync"
	daxbackend "github.com/blackbirdworks/gopherstack/services/dax"
	detectivebackend "github.com/blackbirdworks/gopherstack/services/detective"
	directconnectbackend "github.com/blackbirdworks/gopherstack/services/directconnect"
	directoryservicebackend "github.com/blackbirdworks/gopherstack/services/directoryservice"
	dlmbackend "github.com/blackbirdworks/gopherstack/services/dlm"
	dmsbackend "github.com/blackbirdworks/gopherstack/services/dms"
	docdbbackend "github.com/blackbirdworks/gopherstack/services/docdb"
	ddbbackend "github.com/blackbirdworks/gopherstack/services/dynamodb"
	ddbmodels "github.com/blackbirdworks/gopherstack/services/dynamodb/models"
	dynamodbstreamsbackend "github.com/blackbirdworks/gopherstack/services/dynamodbstreams"
	ec2backend "github.com/blackbirdworks/gopherstack/services/ec2"
	ecrbackend "github.com/blackbirdworks/gopherstack/services/ecr"
	ecsbackend "github.com/blackbirdworks/gopherstack/services/ecs"
	efsbackend "github.com/blackbirdworks/gopherstack/services/efs"
	eksbackend "github.com/blackbirdworks/gopherstack/services/eks"
	elasticachebackend "github.com/blackbirdworks/gopherstack/services/elasticache"
	elasticbeanstalkbackend "github.com/blackbirdworks/gopherstack/services/elasticbeanstalk"
	elasticsearchbackend "github.com/blackbirdworks/gopherstack/services/elasticsearch"
	elbbackend "github.com/blackbirdworks/gopherstack/services/elb"
	elbv2backend "github.com/blackbirdworks/gopherstack/services/elbv2"
	emrbackend "github.com/blackbirdworks/gopherstack/services/emr"
	emrserverlessbackend "github.com/blackbirdworks/gopherstack/services/emrserverless"
	ebbackend "github.com/blackbirdworks/gopherstack/services/eventbridge"
	firehosebackend "github.com/blackbirdworks/gopherstack/services/firehose"
	fisbackend "github.com/blackbirdworks/gopherstack/services/fis"
	forecastbackend "github.com/blackbirdworks/gopherstack/services/forecast"
	fsxbackend "github.com/blackbirdworks/gopherstack/services/fsx"
	glacierbackend "github.com/blackbirdworks/gopherstack/services/glacier"
	gluebackend "github.com/blackbirdworks/gopherstack/services/glue"
	grafanabackend "github.com/blackbirdworks/gopherstack/services/grafana"
	guarddutybackend "github.com/blackbirdworks/gopherstack/services/guardduty"
	iambackend "github.com/blackbirdworks/gopherstack/services/iam"
	identitystorebackend "github.com/blackbirdworks/gopherstack/services/identitystore"
	inspector2backend "github.com/blackbirdworks/gopherstack/services/inspector2"
	iotbackend "github.com/blackbirdworks/gopherstack/services/iot"
	iotanalyticsbackend "github.com/blackbirdworks/gopherstack/services/iotanalytics"
	iotdataplanebackend "github.com/blackbirdworks/gopherstack/services/iotdataplane"
	iotwirelessbackend "github.com/blackbirdworks/gopherstack/services/iotwireless"
	kafkabackend "github.com/blackbirdworks/gopherstack/services/kafka"
	kinesisbackend "github.com/blackbirdworks/gopherstack/services/kinesis"
	kinesisanalyticsbackend "github.com/blackbirdworks/gopherstack/services/kinesisanalytics"
	kinesisanalyticsv2backend "github.com/blackbirdworks/gopherstack/services/kinesisanalyticsv2"
	kmsbackend "github.com/blackbirdworks/gopherstack/services/kms"
	lakeformationbackend "github.com/blackbirdworks/gopherstack/services/lakeformation"
	lambdabackend "github.com/blackbirdworks/gopherstack/services/lambda"
	lightsailbackend "github.com/blackbirdworks/gopherstack/services/lightsail"
	macie2backend "github.com/blackbirdworks/gopherstack/services/macie2"
	managedblockchainbackend "github.com/blackbirdworks/gopherstack/services/managedblockchain"
	mediaconvertbackend "github.com/blackbirdworks/gopherstack/services/mediaconvert"
	medialivebackend "github.com/blackbirdworks/gopherstack/services/medialive"
	mediapackagebackend "github.com/blackbirdworks/gopherstack/services/mediapackage"
	mediastorebackend "github.com/blackbirdworks/gopherstack/services/mediastore"
	mediastoredatabackend "github.com/blackbirdworks/gopherstack/services/mediastoredata"
	mediatailorbackend "github.com/blackbirdworks/gopherstack/services/mediatailor"
	memorydbbackend "github.com/blackbirdworks/gopherstack/services/memorydb"
	mgnbackend "github.com/blackbirdworks/gopherstack/services/mgn"
	mqbackend "github.com/blackbirdworks/gopherstack/services/mq"
	mwaabackend "github.com/blackbirdworks/gopherstack/services/mwaa"
	neptunebackend "github.com/blackbirdworks/gopherstack/services/neptune"
	networkmanagerbackend "github.com/blackbirdworks/gopherstack/services/networkmanager"
	networkmonitorbackend "github.com/blackbirdworks/gopherstack/services/networkmonitor"
	omicsbackend "github.com/blackbirdworks/gopherstack/services/omics"
	opensearchbackend "github.com/blackbirdworks/gopherstack/services/opensearch"
	opsworksbackend "github.com/blackbirdworks/gopherstack/services/opsworks"
	organizationsbackend "github.com/blackbirdworks/gopherstack/services/organizations"
	outpostsbackend "github.com/blackbirdworks/gopherstack/services/outposts"
	personalizebackend "github.com/blackbirdworks/gopherstack/services/personalize"
	pinpointbackend "github.com/blackbirdworks/gopherstack/services/pinpoint"
	pipesbackend "github.com/blackbirdworks/gopherstack/services/pipes"
	pollybackend "github.com/blackbirdworks/gopherstack/services/polly"
	quicksightbackend "github.com/blackbirdworks/gopherstack/services/quicksight"
	rambackend "github.com/blackbirdworks/gopherstack/services/ram"
	rdsbackend "github.com/blackbirdworks/gopherstack/services/rds"
	rdsdatabackend "github.com/blackbirdworks/gopherstack/services/rdsdata"
	redshiftbackend "github.com/blackbirdworks/gopherstack/services/redshift"
	redshiftdatabackend "github.com/blackbirdworks/gopherstack/services/redshiftdata"
	rekognitionbackend "github.com/blackbirdworks/gopherstack/services/rekognition"
	resiliencehubbackend "github.com/blackbirdworks/gopherstack/services/resiliencehub"
	resourcegroupsbackend "github.com/blackbirdworks/gopherstack/services/resourcegroups"
	resourcegroupstaggingapibackend "github.com/blackbirdworks/gopherstack/services/resourcegroupstaggingapi"
	rolesanywherebackend "github.com/blackbirdworks/gopherstack/services/rolesanywhere"
	route53backend "github.com/blackbirdworks/gopherstack/services/route53"
	route53resolverbackend "github.com/blackbirdworks/gopherstack/services/route53resolver"
	s3backend "github.com/blackbirdworks/gopherstack/services/s3"
	s3controlbackend "github.com/blackbirdworks/gopherstack/services/s3control"
	s3tablesbackend "github.com/blackbirdworks/gopherstack/services/s3tables"
	sagemakerbackend "github.com/blackbirdworks/gopherstack/services/sagemaker"
	sagemakerruntimebackend "github.com/blackbirdworks/gopherstack/services/sagemakerruntime"
	schedulerbackend "github.com/blackbirdworks/gopherstack/services/scheduler"
	secretsmanagerbackend "github.com/blackbirdworks/gopherstack/services/secretsmanager"
	securityhubbackend "github.com/blackbirdworks/gopherstack/services/securityhub"
	serverlessrepobackend "github.com/blackbirdworks/gopherstack/services/serverlessrepo"
	servicediscoverybackend "github.com/blackbirdworks/gopherstack/services/servicediscovery"
	sesbackend "github.com/blackbirdworks/gopherstack/services/ses"
	sesv2backend "github.com/blackbirdworks/gopherstack/services/sesv2"
	shieldbackend "github.com/blackbirdworks/gopherstack/services/shield"
	snsbackend "github.com/blackbirdworks/gopherstack/services/sns"
	sqsbackend "github.com/blackbirdworks/gopherstack/services/sqs"
	ssmbackend "github.com/blackbirdworks/gopherstack/services/ssm"
	ssoadminbackend "github.com/blackbirdworks/gopherstack/services/ssoadmin"
	sfnbackend "github.com/blackbirdworks/gopherstack/services/stepfunctions"
	stsbackend "github.com/blackbirdworks/gopherstack/services/sts"
	supportbackend "github.com/blackbirdworks/gopherstack/services/support"
	swfbackend "github.com/blackbirdworks/gopherstack/services/swf"
	textractbackend "github.com/blackbirdworks/gopherstack/services/textract"
	timestreamquerybackend "github.com/blackbirdworks/gopherstack/services/timestreamquery"
	timestreamwritebackend "github.com/blackbirdworks/gopherstack/services/timestreamwrite"
	transcribebackend "github.com/blackbirdworks/gopherstack/services/transcribe"
	transferbackend "github.com/blackbirdworks/gopherstack/services/transfer"
	translatebackend "github.com/blackbirdworks/gopherstack/services/translate"
	verifiedpermissionsbackend "github.com/blackbirdworks/gopherstack/services/verifiedpermissions"
	vpclatticebackend "github.com/blackbirdworks/gopherstack/services/vpclattice"
	wafbackend "github.com/blackbirdworks/gopherstack/services/waf"
	wafv2backend "github.com/blackbirdworks/gopherstack/services/wafv2"
	workmailbackend "github.com/blackbirdworks/gopherstack/services/workmail"
	workspacesbackend "github.com/blackbirdworks/gopherstack/services/workspaces"
	xraybackend "github.com/blackbirdworks/gopherstack/services/xray"

	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
)

const (
	defaultPort              = "8000"
	defaultRegion            = "us-east-1"
	defaultTimeout           = 30 * time.Second
	shutdownTimeout          = 5 * time.Second
	healthCheckTimeout       = 5 * time.Second
	configFilename           = "config.json"
	defaultReadHeaderTimeout = 5 * time.Second
	configDirPerm            = 0o700
	configFilePerm           = 0o600

	// selfSignedValidity is how long a generated self-signed TLS cert is valid.
	selfSignedValidity = 365 * 24 * time.Hour
	// selfSignedSerialBits is the bit-length of the random certificate serial.
	selfSignedSerialBits = 128
	// localhostName is the hostname the self-signed dev certificate is issued for.
	localhostName = "localhost"
	// loopbackIPv4Octet is the first octet of the IPv4 loopback address (127.x).
	loopbackIPv4Octet = 127

	keyMessageField      = "message"
	logLevelDebug        = "debug"
	demoAppName          = "demo-app"
	contentTypeJSON      = "application/json"
	emrServerlessRoleARN = "arn:aws:iam::000000000000:role/EMRServerlessRole"
	envProduction        = "production"
	kinesisServiceName   = "kinesis"
)

// CLI holds all command-line / environment-variable configuration for Gopherstack.
type CLI struct {
	SecretsManager                struct{}            `embed:"" prefix:"secretsmanager-"`
	SQS                           sqsbackend.Settings `embed:"" prefix:"sqs-"`
	SNS                           struct{}            `embed:"" prefix:"sns-"`
	IAM                           struct{}            `embed:"" prefix:"iam-"`
	kinesisHandler                service.Registerable
	athenaHandler                 service.Registerable
	emrserverlessHandler          service.Registerable
	s3tablesHandler               service.Registerable
	grafanaHandler                service.Registerable
	outpostsHandler               service.Registerable
	resiliencehubHandler          service.Registerable
	directconnectHandler          service.Registerable
	mgnHandler                    service.Registerable
	networkmanagerHandler         service.Registerable
	lightsailHandler              service.Registerable
	xrayHandler                   service.Registerable
	wafHandler                    service.Registerable
	wafv2Handler                  service.Registerable
	verifiedPermissionsHandler    service.Registerable
	transferHandler               service.Registerable
	timestreamqueryHandler        service.Registerable
	timestreamwriteHandler        service.Registerable
	textractHandler               service.Registerable
	ssoadminHandler               service.Registerable
	shieldHandler                 service.Registerable
	serverlessrepoHandler         service.Registerable
	elasticacheHandler            service.Registerable
	iotHandler                    service.Registerable
	ddbHandler                    service.Registerable
	s3Handler                     service.Registerable
	ssmHandler                    service.Registerable
	iamHandler                    service.Registerable
	stsHandler                    service.Registerable
	snsHandler                    service.Registerable
	sqsHandler                    service.Registerable
	lambdaHandler                 service.Registerable
	eventBridgeHandler            service.Registerable
	apiGatewayHandler             service.Registerable
	cognitoIDPHandler             service.Registerable
	stepFunctionsHandler          service.Registerable
	cloudWatchHandler             service.Registerable
	cloudFormationHandler         service.Registerable
	kmsHandler                    service.Registerable
	route53Handler                service.Registerable
	sesHandler                    service.Registerable
	sesv2Handler                  service.Registerable
	ec2Handler                    service.Registerable
	elasticsearchHandler          service.Registerable
	openSearchHandler             service.Registerable
	acmHandler                    service.Registerable
	acmpcaHandler                 service.Registerable
	redshiftHandler               service.Registerable
	redshiftServerlessHandler     service.Registerable
	rdsHandler                    service.Registerable
	awsconfigHandler              service.Registerable
	sagemakerRuntimeHandler       service.Registerable
	resourcegroupsHandler         service.Registerable
	resourcegroupstaggingHandler  service.Registerable
	swfHandler                    service.Registerable
	firehoseHandler               service.Registerable
	networkmonitorHandler         service.Registerable
	schedulerHandler              service.Registerable
	servicediscoveryHandler       service.Registerable
	transcribeHandler             service.Registerable
	supportHandler                service.Registerable
	appSyncHandler                service.Registerable
	iotDataPlaneHandler           service.Registerable
	apiGatewayMgmtHandler         service.Registerable
	appConfigDataHandler          service.Registerable
	amplifyHandler                service.Registerable
	autoscalingHandler            service.Registerable
	apiGatewayV2Handler           service.Registerable
	secretsManagerHandler         service.Registerable
	backupHandler                 service.Registerable
	cloudtrailHandler             service.Registerable
	appConfigHandler              service.Registerable
	applicationautoscalingHandler service.Registerable
	batchHandler                  service.Registerable
	bedrockHandler                service.Registerable
	bedrockAgentsHandler          service.Registerable
	bedrockruntimeHandler         service.Registerable
	ceHandler                     service.Registerable
	cloudcontrolHandler           service.Registerable
	cloudFrontHandler             service.Registerable
	codeArtifactHandler           service.Registerable
	codebuildHandler              service.Registerable
	codeCommitHandler             service.Registerable
	codePipelineHandler           service.Registerable
	codeConnectionsHandler        service.Registerable
	codeDeployHandler             service.Registerable
	comprehendHandler             service.Registerable
	dmsHandler                    service.Registerable
	codeStarConnectionsHandler    service.Registerable
	dynamodbStreamsHandler        service.Registerable
	docdbHandler                  service.Registerable
	elasticbeanstalkHandler       service.Registerable
	ecrHandler                    service.Registerable
	ecsHandler                    service.Registerable
	efsHandler                    service.Registerable
	eksHandler                    service.Registerable
	elbHandler                    service.Registerable
	elbv2Handler                  service.Registerable
	route53resolverHandler        service.Registerable
	cloudWatchLogsHandler         service.Registerable
	s3controlHandler              service.Registerable
	cognitoIdentityHandler        service.Registerable
	fisHandler                    service.Registerable
	identitystoreHandler          service.Registerable
	emrHandler                    service.Registerable
	glacierHandler                service.Registerable
	iotwirelessHandler            service.Registerable
	kinesisanalyticsHandler       service.Registerable
	lakeformationHandler          service.Registerable
	glueHandler                   service.Registerable
	guarddutyHandler              service.Registerable
	inspector2Handler             service.Registerable
	iotanalyticsHandler           service.Registerable
	kafkaHandler                  service.Registerable
	kinesisanalyticsv2Handler     service.Registerable
	managedblockchainHandler      service.Registerable
	mediaconvertHandler           service.Registerable
	mqHandler                     service.Registerable
	mediastoreHandler             service.Registerable
	mediastoredataHandler         service.Registerable
	memorydbHandler               service.Registerable
	organizationsHandler          service.Registerable
	mwaaHandler                   service.Registerable
	neptuneHandler                service.Registerable
	pinpointHandler               service.Registerable
	pipesHandler                  service.Registerable
	rdsdataHandler                service.Registerable
	ramHandler                    service.Registerable
	redshiftdataHandler           service.Registerable
	sagemakerHandler              service.Registerable
	ssmClient                     *ssmsdk.Client
	ecsClient                     *ecs.Client
	amplifyClient                 *amplifysdk.Client
	codePipelineSDKClient         *codepipelinesdk.Client
	codeDeployClient              *codedeploysdk.Client
	iotClient                     *iotsdk.Client
	eksClient                     *ekssdk.Client
	appSyncSdkClient              *appsyncsdksvc.Client
	ecrClient                     *ecr.Client
	secretsManagerClient          *secretsmanager.Client
	sqsClient                     *sqssdk.Client
	stsClient                     *stssdk.Client
	ddbClient                     *dynamodb.Client
	faultStore                    *chaos.FaultStore
	snsClient                     *sns.Client
	kmsClient                     *kms.Client
	iamClient                     *iam.Client
	s3Client                      *s3.Client
	globalConfig                  *config.GlobalConfig
	portAlloc                     *portalloc.Allocator
	ElastiCacheEngine             string                          `                                    name:"elasticache-engine"      env:"ELASTICACHE_ENGINE"      default:"embedded"      help:"ElastiCache engine mode: embedded (miniredis), stub, or docker."`                                      //nolint:lll // config struct tags are intentionally verbose
	EC2Provider                   string                          `                                    name:"ec2-provider"            env:"EC2_PROVIDER"            default:"inmemory"      help:"EC2 compute provider: inmemory (stub) or docker (launches real containers as instances)."`             //nolint:lll // config struct tags are intentionally verbose
	EC2DockerImage                string                          `                                    name:"ec2-docker-image"        env:"EC2_DOCKER_IMAGE"        default:"amazonlinux:2" help:"Docker image used by the EC2 docker provider when launching instances."`                               //nolint:lll // config struct tags are intentionally verbose
	EC2DockerNetwork              string                          `                                    name:"ec2-docker-network"      env:"EC2_DOCKER_NETWORK"      default:""              help:"Docker network EC2 docker-provider containers attach to (empty = daemon default bridge)."`             //nolint:lll // config struct tags are intentionally verbose
	EC2DockerSSHHostIP            string                          `                                    name:"ec2-docker-ssh-host-ip"  env:"EC2_DOCKER_SSH_HOST_IP"  default:"127.0.0.1"     help:"Host IP that mapped EC2-docker SSH ports bind to (use 0.0.0.0 to expose externally)."`                 //nolint:lll // config struct tags are intentionally verbose
	Port                          string                          `                                    name:"port"                    env:"PORT"                    default:"8000"          help:"HTTP server port."`                                                                                    //nolint:lll // config struct tags are intentionally verbose
	DataDir                       string                          `                                    name:"data-dir"                env:"GOPHERSTACK_DATA_DIR"    default:""              help:"Directory for persistence data files (default: ~/.gopherstack/data, or /data in containers)."`         //nolint:lll // config struct tags are intentionally verbose
	DNSListenAddr                 string                          `                                    name:"dns-addr"                env:"DNS_ADDR"                default:""              help:"Address for embedded DNS server (e.g. :10053). Empty = disabled."`                                     //nolint:lll // config struct tags are intentionally verbose
	LogLevel                      string                          `                                    name:"log-level"               env:"LOG_LEVEL"               default:"info"          help:"Log level (debug|info|warn|error)."`                                                                   //nolint:lll // config struct tags are intentionally verbose
	Region                        string                          `                                    name:"region"                  env:"REGION"                  default:"us-east-1"     help:"AWS region (also read from AWS_DEFAULT_REGION and AWS_REGION)."`                                       //nolint:lll // config struct tags are intentionally verbose
	OpenSearchEngine              string                          `                                    name:"opensearch-engine"       env:"OPENSEARCH_ENGINE"       default:"stub"          help:"OpenSearch engine mode: stub (API-only) or docker."`                                                   //nolint:lll // config struct tags are intentionally verbose
	ElasticsearchEngine           string                          `                                    name:"elasticsearch-engine"    env:"ELASTICSEARCH_ENGINE"    default:"stub"          help:"Elasticsearch engine mode: stub (API-only) or docker."`                                                //nolint:lll // config struct tags are intentionally verbose
	DNSResolveIP                  string                          `                                    name:"dns-resolve-ip"          env:"DNS_RESOLVE_IP"          default:"127.0.0.1"     help:"IP address synthetic hostnames resolve to."`                                                           //nolint:lll // config struct tags are intentionally verbose
	AccountID                     string                          `                                    name:"account-id"              env:"ACCOUNT_ID"              default:"000000000000"  help:"Mock AWS account ID used in ARNs."`                                                                    //nolint:lll // config struct tags are intentionally verbose
	TLSCertFile                   string                          `                                    name:"tls-cert"                env:"TLS_CERT"                default:""              help:"Path to a TLS certificate (PEM). Enables an HTTPS listener; requires --tls-key. Empty = HTTP only."`   //nolint:lll // config struct tags are intentionally verbose
	TLSKeyFile                    string                          `                                    name:"tls-key"                 env:"TLS_KEY"                 default:""              help:"Path to a TLS private key (PEM). Required with --tls-cert."`                                           //nolint:lll // config struct tags are intentionally verbose
	SigV4Secret                   string                          `                                    name:"sigv4-secret"            env:"SIGV4_SECRET"            default:"test"          help:"Secret access key SigV4 validation signs against (used only when --validate-sigv4 is set)."`           //nolint:lll // config struct tags are intentionally verbose
	InitScripts                   []string                        `                                    name:"init-script"             env:"INIT_SCRIPTS"                                    help:"Shell scripts to run on startup (may be specified multiple times)."`                                   //nolint:lll // config struct tags are intentionally verbose
	S3InitBuckets                 []string                        `                                    name:"s3-bucket"               env:"S3_BUCKETS"                                      help:"S3 bucket names to create on startup (may be specified multiple times or as a comma-separated list)."` //nolint:lll // config struct tags are intentionally verbose
	S3                            s3backend.Settings              `embed:"" prefix:"s3-"`
	CosmosDB                      cosmosdbbackend.Settings        `embed:"" prefix:"cosmosdb-"`
	Lambda                        lambdabackend.Settings          `embed:"" prefix:"lambda-"`
	DynamoDB                      ddbbackend.Settings             `embed:"" prefix:"dynamodb-"`
	EC2                           ec2backend.Settings             `embed:"" prefix:"ec2-"`
	Batch                         batchbackend.Settings           `embed:"" prefix:"batch-"`
	StepFunctions                 sfnbackend.Settings             `embed:"" prefix:"stepfunctions-"`
	CodeBuild                     codebuildbackend.Settings       `embed:"" prefix:"codebuild-"`
	Backup                        backupbackend.Settings          `embed:"" prefix:"backup-"`
	SSM                           ssmbackend.Settings             `embed:"" prefix:"ssm-"`
	XRay                          xraybackend.Settings            `embed:"" prefix:"xray-"`
	SES                           sesbackend.Settings             `embed:"" prefix:"ses-"`
	FIS                           fisbackend.Settings             `embed:"" prefix:"fis-"`
	EMR                           emrbackend.Settings             `embed:"" prefix:"emr-"`
	Athena                        athenabackend.Settings          `embed:"" prefix:"athena-"`
	CloudWatchLogs                cwlogsbackend.Settings          `embed:"" prefix:"cloudwatchlogs-"`
	KMS                           kmsbackend.Settings             `embed:"" prefix:"kms-"`
	Kinesis                       kinesisbackend.Settings         `embed:"" prefix:"kinesis-"`
	STS                           stsbackend.Settings             `embed:"" prefix:"sts-"`
	AzureBlob                     azureblobbackend.Settings       `embed:"" prefix:"azure-blob-"`
	AzureQueue                    azurequeuebackend.Settings      `embed:"" prefix:"azure-queue-"`
	AzureTable                    azuretablebackend.Settings      `embed:"" prefix:"azure-table-"`
	AzureServiceBus               azureservicebusbackend.Settings `embed:"" prefix:"azure-servicebus-"`
	PortRangeStart                int                             `                                    name:"port-range-start"        env:"PORT_RANGE_START"        default:"10000"         help:"Start of the port range for resource endpoints."`                                                                                                                                              //nolint:lll // config struct tags are intentionally verbose
	PortRangeEnd                  int                             `                                    name:"port-range-end"          env:"PORT_RANGE_END"          default:"10100"         help:"End (exclusive) of the port range for resource endpoints."`                                                                                                                                    //nolint:lll // config struct tags are intentionally verbose
	EC2DockerSSHPortMin           int                             `                                    name:"ec2-docker-ssh-port-min" env:"EC2_DOCKER_SSH_PORT_MIN" default:"0"             help:"Lower bound of the host TCP port range used to map EC2-docker SSH (0 = let Docker pick)."`                                                                                                     //nolint:lll // config struct tags are intentionally verbose
	EC2DockerSSHPortMax           int                             `                                    name:"ec2-docker-ssh-port-max" env:"EC2_DOCKER_SSH_PORT_MAX" default:"0"             help:"Upper bound of the host TCP port range used to map EC2-docker SSH."`                                                                                                                           //nolint:lll // config struct tags are intentionally verbose
	InitScriptTimeout             time.Duration                   `                                    name:"init-timeout"            env:"INIT_TIMEOUT"            default:"30s"           help:"Per-script timeout for init hooks."`                                                                                                                                                           //nolint:lll // config struct tags are intentionally verbose
	JanitorTimeout                time.Duration                   `                                    name:"janitor-timeout"         env:"JANITOR_TIMEOUT"         default:"30s"           help:"Per-task timeout for janitor operations (TTL sweeps, table cleaners, etc.). Zero disables per-task timeouts. Higher values prevent deadlocks; lower values keep the janitor loop responsive."` //nolint:lll // config struct tags are intentionally verbose
	LatencyMs                     int                             `                                    name:"latency-ms"              env:"LATENCY_MS"              default:"0"             help:"Inject random latency [0,N) ms per request (0 = disabled). Values near the 30 s write timeout may cause connection errors."`                                                                   //nolint:lll // config struct tags are intentionally verbose
	AutoPurgeTTL                  time.Duration                   `                                    name:"auto-purge-ttl"          env:"AUTO_PURGE_TTL"                                  help:"If set, automatically reset all services on a timer based on the TTL (e.g., 10m)."`                                                                                                            //nolint:lll // config struct tags are intentionally verbose
	EnforceIAM                    bool                            `                                    name:"enforce-iam"             env:"GOPHERSTACK_ENFORCE_IAM" default:"false"         help:"Enable IAM policy enforcement. When true, every AWS API request is evaluated against attached IAM policies."`                                                                                  //nolint:lll // config struct tags are intentionally verbose
	Persist                       bool                            `                                    name:"persist"                 env:"PERSIST"                 default:"false"         help:"Enable snapshot-based persistence across restarts."`                                                                                                                                           //nolint:lll // config struct tags are intentionally verbose
	Demo                          bool                            `                                    name:"demo"                    env:"DEMO"                    default:"false"         help:"Load demo data on startup."`                                                                                                                                                                   //nolint:lll // config struct tags are intentionally verbose
	TLS                           bool                            `                                    name:"tls"                     env:"TLS"                     default:"false"         help:"Serve over HTTPS. With --tls-cert/--tls-key uses those files; otherwise a self-signed certificate is generated on demand."`                                                                    //nolint:lll // config struct tags are intentionally verbose
	ValidateSigV4                 bool                            `                                    name:"validate-sigv4"          env:"VALIDATE_SIGV4"          default:"false"         help:"Cryptographically validate AWS SigV4 request signatures (opt-in). Signed requests whose signature does not match --sigv4-secret are rejected."`                                                //nolint:lll // config struct tags are intentionally verbose
}

// GetGlobalConfig returns the centralised account ID and region (config.Provider).
func (c *CLI) GetGlobalConfig() *config.GlobalConfig {
	if c.globalConfig == nil {
		c.globalConfig = config.NewGlobalConfig(
			c.AccountID,
			c.Region,
			c.LatencyMs,
			c.JanitorTimeout,
			c.EnforceIAM,
			c.AutoPurgeTTL,
		)
	}

	return c.globalConfig
}

// resolvedDataDir returns the effective data directory for persistence.
func (c *CLI) resolvedDataDir() string {
	if c.DataDir != "" {
		return c.DataDir
	}

	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "/data"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".gopherstack", "data")
	}

	return filepath.Join(home, ".gopherstack", "data")
}

// createPersistenceStore creates a FileStore using the resolved data directory.
func (c *CLI) createPersistenceStore() (*persistence.FileStore, error) {
	return persistence.NewFileStore(c.resolvedDataDir())
}

func (c *CLI) GetDynamoDBSettings() ddbbackend.Settings {
	return c.DynamoDB
}

// GetS3Settings returns S3 settings (s3.ConfigProvider).
func (c *CLI) GetS3Settings() s3backend.Settings {
	return c.S3
}

// GetAzureBlobSettings returns Azure Blob settings (azureblob.ConfigProvider).
func (c *CLI) GetAzureBlobSettings() azureblobbackend.Settings {
	return c.AzureBlob
}

// GetAzureQueueSettings returns Azure Queue settings (azurequeue.ConfigProvider).
func (c *CLI) GetAzureQueueSettings() azurequeuebackend.Settings {
	return c.AzureQueue
}

// GetAzureTableSettings returns Azure Table settings (azuretable.ConfigProvider).
func (c *CLI) GetAzureTableSettings() azuretablebackend.Settings {
	return c.AzureTable
}

// GetCosmosDBSettings returns Cosmos DB settings (cosmosdb.ConfigProvider).
func (c *CLI) GetCosmosDBSettings() cosmosdbbackend.Settings {
	return c.CosmosDB
}

// GetAzureServiceBusSettings returns Azure Service Bus settings
// (azureservicebus.ConfigProvider).
func (c *CLI) GetAzureServiceBusSettings() azureservicebusbackend.Settings {
	return c.AzureServiceBus
}

// GetS3Endpoint returns the configured S3 endpoint (s3.ConfigProvider).
func (c *CLI) GetS3Endpoint() string {
	s3Port := strings.TrimPrefix(c.Port, ":")

	return "localhost:" + s3Port
}

// GetEndpoint returns the base HTTP endpoint URL for this Gopherstack instance.
func (c *CLI) GetEndpoint() string {
	port := strings.TrimPrefix(c.Port, ":")

	return "http://localhost:" + port
}

// GetLambdaSettings returns Lambda settings (lambda.SettingsProvider).
func (c *CLI) GetLambdaSettings() lambdabackend.Settings {
	return c.Lambda
}

// GetStepFunctionsSettings returns Step Functions settings (stepfunctions.SettingsProvider).
func (c *CLI) GetStepFunctionsSettings() sfnbackend.Settings {
	return c.StepFunctions
}

// GetSSMSettings returns SSM settings (ssm.ConfigProvider).
func (c *CLI) GetSSMSettings() ssmbackend.Settings {
	return c.SSM
}

// GetKMSSettings returns KMS settings (kms.ConfigProvider).
func (c *CLI) GetKMSSettings() kmsbackend.Settings {
	return c.KMS
}

// GetSTSSettings returns STS settings (sts.ConfigProvider).
func (c *CLI) GetSTSSettings() stsbackend.Settings {
	return c.STS
}

// PersistableConfig holds settings that can be saved to disk.
type PersistableConfig struct {
	ElasticsearchEngine string                    `json:"elasticsearch_engine"`
	ElastiCacheEngine   string                    `json:"elasticache_engine"`
	Region              string                    `json:"region"`
	LogLevel            string                    `json:"log_level"`
	Port                string                    `json:"port"`
	OpenSearchEngine    string                    `json:"opensearch_engine"`
	AccountID           string                    `json:"account_id"`
	DataDir             string                    `json:"data_dir"`
	DNSListenAddr       string                    `json:"dns_listen_addr"`
	DNSResolveIP        string                    `json:"dns_resolve_ip"`
	S3                  s3backend.Settings        `json:"s3"`
	Lambda              lambdabackend.Settings    `json:"lambda"`
	DynamoDB            ddbbackend.Settings       `json:"dynamodb"`
	Batch               batchbackend.Settings     `json:"batch"`
	EC2                 ec2backend.Settings       `json:"ec2"`
	CodeBuild           codebuildbackend.Settings `json:"codebuild"`
	FIS                 fisbackend.Settings       `json:"fis"`
	Athena              athenabackend.Settings    `json:"athena"`
	EMR                 emrbackend.Settings       `json:"emr"`
	SES                 sesbackend.Settings       `json:"ses"`
	SSM                 ssmbackend.Settings       `json:"ssm"`
	XRay                xraybackend.Settings      `json:"xray"`
	Backup              backupbackend.Settings    `json:"backup"`
	PortRangeEnd        int                       `json:"port_range_end"`
	InitScriptTimeout   time.Duration             `json:"init_script_timeout"`
	LatencyMs           int                       `json:"latency_ms"`
	PortRangeStart      int                       `json:"port_range_start"`
	JanitorTimeout      time.Duration             `json:"janitor_timeout"`
	CloudWatchLogs      cwlogsbackend.Settings    `json:"cloudwatchlogs"`
	STS                 stsbackend.Settings       `json:"sts"`
	StepFunctions       sfnbackend.Settings       `json:"stepfunctions"`
	AutoPurgeTTL        time.Duration             `json:"auto_purge_ttl"`
	Kinesis             kinesisbackend.Settings   `json:"kinesis"`
	KMS                 kmsbackend.Settings       `json:"kms"`
	EnforceIAM          bool                      `json:"enforce_iam"`
	Demo                bool                      `json:"demo"`
	Persist             bool                      `json:"persist"`
}

// GetSettings returns the dashboard configuration settings.
func (c *CLI) GetSettings() dashboard.Settings {
	return dashboard.Settings{
		AccountID:           c.AccountID,
		Region:              c.Region,
		LatencyMs:           c.LatencyMs,
		EnforceIAM:          c.EnforceIAM,
		AutoPurgeTTL:        c.AutoPurgeTTL,
		JanitorTimeout:      c.JanitorTimeout,
		LogLevel:            c.LogLevel,
		Port:                c.Port,
		DNSListenAddr:       c.DNSListenAddr,
		DNSResolveIP:        c.DNSResolveIP,
		OpenSearchEngine:    c.OpenSearchEngine,
		ElasticsearchEngine: c.ElasticsearchEngine,
		ElastiCacheEngine:   c.ElastiCacheEngine,
		Persist:             c.Persist,
		Demo:                c.Demo,
		DataDir:             c.DataDir,
		PortRangeStart:      c.PortRangeStart,
		PortRangeEnd:        c.PortRangeEnd,
		InitScriptTimeout:   c.InitScriptTimeout,

		// Service settings
		S3: dashboard.S3Settings{
			DefaultRegion:       c.S3.DefaultRegion,
			JanitorInterval:     c.S3.JanitorInterval,
			CompressionMinBytes: c.S3.CompressionMinBytes,
		},
		Lambda: dashboard.LambdaSettings{
			DockerHost:       c.Lambda.DockerHost,
			ContainerRuntime: c.Lambda.ContainerRuntime,
			PoolSize:         c.Lambda.PoolSize,
			IdleTimeout:      c.Lambda.IdleTimeout,
			MaxRuntimes:      c.Lambda.MaxRuntimes,
		},
		DynamoDB: dashboard.DynamoDBSettings{
			DefaultRegion:     c.DynamoDB.DefaultRegion,
			JanitorInterval:   c.DynamoDB.JanitorInterval,
			CreateDelay:       c.DynamoDB.CreateDelay,
			EnforceThroughput: c.DynamoDB.EnforceThroughput,
		},
		EC2: dashboard.EC2Settings{
			JanitorInterval:  c.EC2.JanitorInterval,
			TerminatedTTL:    c.EC2.TerminatedTTL,
			CancelledSpotTTL: c.EC2.CancelledSpotTTL,
		},
		Backup: dashboard.BackupSettings{
			JanitorInterval: c.Backup.JanitorInterval,
			JobTTL:          c.Backup.JobTTL,
		},
		STS: dashboard.STSSettings{
			JanitorInterval: c.STS.JanitorInterval,
		},
		XRay: dashboard.XRaySettings{
			JanitorInterval: c.XRay.JanitorInterval,
			TraceTTL:        c.XRay.TraceTTL,
		},
		SSM: dashboard.SSMSettings{
			JanitorInterval: c.SSM.JanitorInterval,
			CommandTTL:      c.SSM.CommandTTL,
		},
		CodeBuild: dashboard.CodeBuildSettings{
			JanitorInterval: c.CodeBuild.JanitorInterval,
			BuildTTL:        c.CodeBuild.BuildTTL,
		},
		CloudWatchLogs: dashboard.CloudWatchLogsSettings{
			JanitorInterval:  c.CloudWatchLogs.JanitorInterval,
			MaxRetentionDays: c.CloudWatchLogs.MaxRetentionDays,
		},
		SES: dashboard.SESSettings{
			JanitorInterval: c.SES.JanitorInterval,
			EmailTTL:        c.SES.EmailTTL,
		},
		Batch: dashboard.BatchSettings{
			JanitorInterval:   c.Batch.JanitorInterval,
			InactiveJobDefTTL: c.Batch.InactiveJobDefTTL,
			CompletedJobTTL:   c.Batch.CompletedJobTTL,
		},
		FIS: dashboard.FISSettings{
			JanitorInterval: c.FIS.JanitorInterval,
			ExperimentTTL:   c.FIS.ExperimentTTL,
		},
		EMR: dashboard.EMRSettings{
			JanitorInterval: c.EMR.JanitorInterval,
			TerminatedTTL:   c.EMR.TerminatedTTL,
		},
		Athena: dashboard.AthenaSettings{
			JanitorInterval: c.Athena.JanitorInterval,
			ExecutionTTL:    c.Athena.ExecutionTTL,
		},
		Kinesis: dashboard.KinesisSettings{
			JanitorInterval: c.Kinesis.JanitorInterval,
		},
		KMS: dashboard.KMSSettings{
			JanitorInterval: c.KMS.JanitorInterval,
		},
		StepFunctions: dashboard.StepFunctionsSettings{
			JanitorInterval:    c.StepFunctions.JanitorInterval,
			ExecutionRetention: c.StepFunctions.ExecutionRetention,
		},
	}
}

// UpdateSettings updates both the CLI state and the shared GlobalConfig pointer.
func (c *CLI) UpdateSettings(s dashboard.Settings) {
	c.AccountID = s.AccountID
	c.Region = s.Region
	c.LatencyMs = s.LatencyMs
	c.EnforceIAM = s.EnforceIAM
	c.AutoPurgeTTL = s.AutoPurgeTTL
	c.JanitorTimeout = s.JanitorTimeout
	c.LogLevel = s.LogLevel
	c.Port = s.Port
	c.DNSListenAddr = s.DNSListenAddr
	c.DNSResolveIP = s.DNSResolveIP
	c.OpenSearchEngine = s.OpenSearchEngine
	c.ElasticsearchEngine = s.ElasticsearchEngine
	c.ElastiCacheEngine = s.ElastiCacheEngine
	c.Persist = s.Persist
	c.Demo = s.Demo
	c.DataDir = s.DataDir
	c.PortRangeStart = s.PortRangeStart
	c.PortRangeEnd = s.PortRangeEnd
	c.InitScriptTimeout = s.InitScriptTimeout

	c.applyServiceSettings(s)

	if c.globalConfig != nil {
		c.globalConfig.Update(
			s.AccountID,
			s.Region,
			s.LatencyMs,
			s.JanitorTimeout,
			s.EnforceIAM,
			s.AutoPurgeTTL,
		)
	}
}

// applyServiceSettings copies per-service settings from s into c.
func (c *CLI) applyServiceSettings(s dashboard.Settings) {
	c.S3.DefaultRegion = s.S3.DefaultRegion
	c.S3.JanitorInterval = s.S3.JanitorInterval
	c.S3.CompressionMinBytes = s.S3.CompressionMinBytes

	c.Lambda.DockerHost = s.Lambda.DockerHost
	c.Lambda.ContainerRuntime = s.Lambda.ContainerRuntime
	c.Lambda.PoolSize = s.Lambda.PoolSize
	c.Lambda.IdleTimeout = s.Lambda.IdleTimeout
	c.Lambda.MaxRuntimes = s.Lambda.MaxRuntimes

	c.DynamoDB.DefaultRegion = s.DynamoDB.DefaultRegion
	c.DynamoDB.JanitorInterval = s.DynamoDB.JanitorInterval
	c.DynamoDB.CreateDelay = s.DynamoDB.CreateDelay
	c.DynamoDB.EnforceThroughput = s.DynamoDB.EnforceThroughput

	c.EC2.JanitorInterval = s.EC2.JanitorInterval
	c.EC2.TerminatedTTL = s.EC2.TerminatedTTL
	c.EC2.CancelledSpotTTL = s.EC2.CancelledSpotTTL

	c.Backup.JanitorInterval = s.Backup.JanitorInterval
	c.Backup.JobTTL = s.Backup.JobTTL

	c.STS.JanitorInterval = s.STS.JanitorInterval

	c.XRay.JanitorInterval = s.XRay.JanitorInterval
	c.XRay.TraceTTL = s.XRay.TraceTTL

	c.SSM.JanitorInterval = s.SSM.JanitorInterval
	c.SSM.CommandTTL = s.SSM.CommandTTL

	c.CodeBuild.JanitorInterval = s.CodeBuild.JanitorInterval
	c.CodeBuild.BuildTTL = s.CodeBuild.BuildTTL

	c.CloudWatchLogs.JanitorInterval = s.CloudWatchLogs.JanitorInterval
	c.CloudWatchLogs.MaxRetentionDays = s.CloudWatchLogs.MaxRetentionDays

	c.SES.JanitorInterval = s.SES.JanitorInterval
	c.SES.EmailTTL = s.SES.EmailTTL

	c.Batch.JanitorInterval = s.Batch.JanitorInterval
	c.Batch.InactiveJobDefTTL = s.Batch.InactiveJobDefTTL
	c.Batch.CompletedJobTTL = s.Batch.CompletedJobTTL

	c.FIS.JanitorInterval = s.FIS.JanitorInterval
	c.FIS.ExperimentTTL = s.FIS.ExperimentTTL

	c.EMR.JanitorInterval = s.EMR.JanitorInterval
	c.EMR.TerminatedTTL = s.EMR.TerminatedTTL

	c.Athena.JanitorInterval = s.Athena.JanitorInterval
	c.Athena.ExecutionTTL = s.Athena.ExecutionTTL

	c.Kinesis.JanitorInterval = s.Kinesis.JanitorInterval
	c.KMS.JanitorInterval = s.KMS.JanitorInterval

	c.StepFunctions.JanitorInterval = s.StepFunctions.JanitorInterval
	c.StepFunctions.ExecutionRetention = s.StepFunctions.ExecutionRetention
}

func (c *CLI) SaveConfig() error {
	if c.Demo {
		return nil
	}
	p := filepath.Join(c.resolvedDataDir(), configFilename)
	dir := filepath.Dir(p)

	if err := os.MkdirAll(dir, configDirPerm); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	cfg := PersistableConfig{
		AccountID:           c.AccountID,
		Region:              c.Region,
		LatencyMs:           c.LatencyMs,
		EnforceIAM:          c.EnforceIAM,
		AutoPurgeTTL:        c.AutoPurgeTTL,
		JanitorTimeout:      c.JanitorTimeout,
		LogLevel:            c.LogLevel,
		PortRangeStart:      c.PortRangeStart,
		PortRangeEnd:        c.PortRangeEnd,
		Port:                c.Port,
		DNSListenAddr:       c.DNSListenAddr,
		DNSResolveIP:        c.DNSResolveIP,
		OpenSearchEngine:    c.OpenSearchEngine,
		ElasticsearchEngine: c.ElasticsearchEngine,
		ElastiCacheEngine:   c.ElastiCacheEngine,
		Persist:             c.Persist,
		Demo:                c.Demo,
		DataDir:             c.DataDir,
		InitScriptTimeout:   c.InitScriptTimeout,

		// Service settings
		S3:             c.S3,
		Lambda:         c.Lambda,
		DynamoDB:       c.DynamoDB,
		Backup:         c.Backup,
		STS:            c.STS,
		EC2:            c.EC2,
		XRay:           c.XRay,
		SSM:            c.SSM,
		CodeBuild:      c.CodeBuild,
		CloudWatchLogs: c.CloudWatchLogs,
		SES:            c.SES,
		Batch:          c.Batch,
		FIS:            c.FIS,
		EMR:            c.EMR,
		Athena:         c.Athena,
		Kinesis:        c.Kinesis,
		KMS:            c.KMS,
		StepFunctions:  c.StepFunctions,
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(p, data, configFilePerm)
}

// LoadConfig loads the configuration from disk if it exists.
func (c *CLI) LoadConfig() error {
	if c.Demo {
		return nil
	}
	p := filepath.Join(c.resolvedDataDir(), configFilename)
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return fmt.Errorf("failed to read config: %w", err)
	}

	var cfg PersistableConfig
	if unmarshalErr := json.Unmarshal(data, &cfg); unmarshalErr != nil {
		return fmt.Errorf("failed to unmarshal config: %w", unmarshalErr)
	}

	// Apply loaded settings to CLI
	c.AccountID = cfg.AccountID
	c.Region = cfg.Region
	c.LatencyMs = cfg.LatencyMs
	c.EnforceIAM = cfg.EnforceIAM
	c.AutoPurgeTTL = cfg.AutoPurgeTTL
	c.JanitorTimeout = cfg.JanitorTimeout
	c.LogLevel = cfg.LogLevel
	c.PortRangeStart = cfg.PortRangeStart
	c.PortRangeEnd = cfg.PortRangeEnd
	c.Port = cfg.Port
	c.DNSListenAddr = cfg.DNSListenAddr
	c.DNSResolveIP = cfg.DNSResolveIP
	c.OpenSearchEngine = cfg.OpenSearchEngine
	c.ElasticsearchEngine = cfg.ElasticsearchEngine
	c.ElastiCacheEngine = cfg.ElastiCacheEngine
	c.Persist = cfg.Persist
	c.Demo = cfg.Demo
	c.DataDir = cfg.DataDir
	c.InitScriptTimeout = cfg.InitScriptTimeout

	// Service settings
	c.S3 = cfg.S3
	c.Lambda = cfg.Lambda
	c.DynamoDB = cfg.DynamoDB
	c.Backup = cfg.Backup
	c.STS = cfg.STS
	c.EC2 = cfg.EC2
	c.XRay = cfg.XRay
	c.SSM = cfg.SSM
	c.CodeBuild = cfg.CodeBuild
	c.CloudWatchLogs = cfg.CloudWatchLogs
	c.SES = cfg.SES
	c.Batch = cfg.Batch
	c.FIS = cfg.FIS
	c.EMR = cfg.EMR
	c.Athena = cfg.Athena
	c.Kinesis = cfg.Kinesis
	c.KMS = cfg.KMS
	c.StepFunctions = cfg.StepFunctions

	return nil
}

// GetAthenaSettings returns Athena settings (athena.ConfigProvider).
func (c *CLI) GetAthenaSettings() athenabackend.Settings {
	return c.Athena
}

// GetBackupSettings returns Backup settings (backup.ConfigProvider).
func (c *CLI) GetBackupSettings() backupbackend.Settings {
	return c.Backup
}

// GetBatchSettings returns Batch settings (batch.ConfigProvider).
func (c *CLI) GetBatchSettings() batchbackend.Settings {
	return c.Batch
}

// GetCloudWatchLogsSettings returns CloudWatch Logs settings (cloudwatchlogs.ConfigProvider).
func (c *CLI) GetCloudWatchLogsSettings() cwlogsbackend.Settings {
	return c.CloudWatchLogs
}

// GetCodeBuildSettings returns CodeBuild settings (codebuild.ConfigProvider).
func (c *CLI) GetCodeBuildSettings() codebuildbackend.Settings {
	return c.CodeBuild
}

// GetEC2Settings returns EC2 settings (ec2.ConfigProvider).
func (c *CLI) GetEC2Settings() ec2backend.Settings {
	return c.EC2
}

// GetEC2ComputeProvider returns the configured EC2 compute provider name
// ("inmemory" or "docker").
func (c *CLI) GetEC2ComputeProvider() string {
	if c.EC2Provider == "" {
		return "inmemory"
	}

	return c.EC2Provider
}

// GetEC2DockerComputeConfig returns the docker provider configuration for the
// EC2 service. The returned value is consumed by ec2.Provider.Init when the
// compute provider is "docker".
func (c *CLI) GetEC2DockerComputeConfig() ec2backend.DockerComputeConfig {
	return ec2backend.DockerComputeConfig{
		Image:      c.EC2DockerImage,
		Network:    c.EC2DockerNetwork,
		SSHHostIP:  c.EC2DockerSSHHostIP,
		Region:     c.Region,
		SSHPortMin: c.EC2DockerSSHPortMin,
		SSHPortMax: c.EC2DockerSSHPortMax,
	}
}

// GetEMRSettings returns EMR settings (emr.ConfigProvider).
func (c *CLI) GetEMRSettings() emrbackend.Settings {
	return c.EMR
}

// GetFISSettings returns FIS settings (fis.ConfigProvider).
func (c *CLI) GetFISSettings() fisbackend.Settings {
	return c.FIS
}

// GetKinesisSettings returns Kinesis settings (kinesis.ConfigProvider).
func (c *CLI) GetKinesisSettings() kinesisbackend.Settings {
	return c.Kinesis
}

// GetSESSettings returns SES settings (ses.ConfigProvider).
func (c *CLI) GetSESSettings() sesbackend.Settings {
	return c.SES
}

// GetXRaySettings returns X-Ray settings (xray.ConfigProvider).
func (c *CLI) GetXRaySettings() xraybackend.Settings {
	return c.XRay
}

// GetDynamoDBClient returns the SDK client for DynamoDB (dashboard.AWSSDKProvider).
func (c *CLI) GetDynamoDBClient() *dynamodb.Client { return c.ddbClient }

// GetS3Client returns the SDK client for S3 (dashboard.AWSSDKProvider).
func (c *CLI) GetS3Client() *s3.Client { return c.s3Client }

// GetSSMClient returns the SDK client for SSM (dashboard.AWSSDKProvider).
func (c *CLI) GetSSMClient() *ssmsdk.Client { return c.ssmClient }

// GetSTSClient returns the SDK client for STS (dashboard.AWSSDKProvider).
func (c *CLI) GetSTSClient() *stssdk.Client { return c.stsClient }

// GetSQSClient returns the SDK client for SQS (dashboard.AWSSDKProvider).
func (c *CLI) GetSQSClient() *sqssdk.Client { return c.sqsClient }

// GetDynamoDBHandler returns the DynamoDB handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetDynamoDBHandler() service.Registerable { return c.ddbHandler }

// GetS3Handler returns the S3 handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetS3Handler() service.Registerable { return c.s3Handler }

// GetSSMHandler returns the SSM handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetSSMHandler() service.Registerable { return c.ssmHandler }

// GetIAMHandler returns the IAM handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetIAMHandler() service.Registerable { return c.iamHandler }

// GetSTSHandler returns the STS handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetSTSHandler() service.Registerable { return c.stsHandler }

// GetSNSHandler returns the SNS handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetSNSHandler() service.Registerable { return c.snsHandler }

// GetSQSHandler returns the SQS handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetSQSHandler() service.Registerable { return c.sqsHandler }

// GetKMSHandler returns the KMS handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetKMSHandler() service.Registerable { return c.kmsHandler }

// GetSecretsManagerHandler returns the Secrets Manager handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetSecretsManagerHandler() service.Registerable { return c.secretsManagerHandler }

// GetLambdaHandler returns the Lambda handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetLambdaHandler() service.Registerable { return c.lambdaHandler }

// GetEventBridgeHandler returns the EventBridge handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetEventBridgeHandler() service.Registerable { return c.eventBridgeHandler }

// GetAPIGatewayHandler returns the API Gateway handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetAPIGatewayHandler() service.Registerable { return c.apiGatewayHandler }

// GetCloudWatchLogsHandler returns the CloudWatch Logs handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetCloudWatchLogsHandler() service.Registerable { return c.cloudWatchLogsHandler }

// GetStepFunctionsHandler returns the Step Functions handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetStepFunctionsHandler() service.Registerable { return c.stepFunctionsHandler }

// GetCloudWatchHandler returns the CloudWatch handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetCloudWatchHandler() service.Registerable { return c.cloudWatchHandler }

// GetCloudFormationHandler returns the CloudFormation handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetCloudFormationHandler() service.Registerable { return c.cloudFormationHandler }

// GetKinesisHandler returns the Kinesis handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetKinesisHandler() service.Registerable { return c.kinesisHandler }

// GetElastiCacheHandler returns the ElastiCache handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetElastiCacheHandler() service.Registerable { return c.elasticacheHandler }

// GetElastiCacheEngine returns the ElastiCache engine mode (elasticache.EngineConfig).
func (c *CLI) GetElastiCacheEngine() string { return c.ElastiCacheEngine }

// GetRoute53Handler returns the Route 53 handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetRoute53Handler() service.Registerable { return c.route53Handler }

// GetSESHandler returns the SES handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetSESHandler() service.Registerable { return c.sesHandler }

// GetSESv2Handler returns the SES v2 handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetSESv2Handler() service.Registerable { return c.sesv2Handler }

// GetEC2Handler returns the EC2 handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetEC2Handler() service.Registerable { return c.ec2Handler }

// GetElasticsearchEngine returns the Elasticsearch engine mode (elasticsearch.EngineConfig).
func (c *CLI) GetElasticsearchEngine() string { return c.ElasticsearchEngine }

// GetElasticsearchHandler returns the Elasticsearch handler.
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetElasticsearchHandler() service.Registerable { return c.elasticsearchHandler }

// GetOpenSearchEngine returns the OpenSearch engine mode (opensearch.EngineConfig).
func (c *CLI) GetOpenSearchEngine() string { return c.OpenSearchEngine }

// GetOpenSearchHandler returns the OpenSearch handler.
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetOpenSearchHandler() service.Registerable { return c.openSearchHandler }

// GetACMHandler returns the ACM handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetACMHandler() service.Registerable { return c.acmHandler }

// GetACMPCAHandler returns the ACM PCA handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetACMPCAHandler() service.Registerable { return c.acmpcaHandler }

// GetRedshiftHandler returns the Redshift handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetRedshiftHandler() service.Registerable { return c.redshiftHandler }

// GetRedshiftServerlessHandler returns the Redshift Serverless handler.
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetRedshiftServerlessHandler() service.Registerable { return c.redshiftServerlessHandler }

// GetRDSHandler returns the RDS handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetRDSHandler() service.Registerable { return c.rdsHandler }

// GetAWSConfigHandler returns the AWS Config handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetAWSConfigHandler() service.Registerable { return c.awsconfigHandler }

// GetS3ControlHandler returns the S3 Control handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetS3ControlHandler() service.Registerable { return c.s3controlHandler }

// GetResourceGroupsHandler returns the Resource Groups handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetResourceGroupsHandler() service.Registerable { return c.resourcegroupsHandler }

// GetResourceGroupsTaggingHandler returns the Resource Groups Tagging API handler.
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetResourceGroupsTaggingHandler() service.Registerable {
	return c.resourcegroupstaggingHandler
}

// GetSWFHandler returns the SWF handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetSWFHandler() service.Registerable { return c.swfHandler }

// GetFirehoseHandler returns the Firehose handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetFirehoseHandler() service.Registerable { return c.firehoseHandler }

// GetNetworkMonitorHandler returns the NetworkMonitor handler.
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetNetworkMonitorHandler() service.Registerable { return c.networkmonitorHandler }

// GetSchedulerHandler returns the Scheduler handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetSchedulerHandler() service.Registerable { return c.schedulerHandler }

// GetRoute53ResolverHandler returns the Route53Resolver handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetRoute53ResolverHandler() service.Registerable { return c.route53resolverHandler }

// GetTranscribeHandler returns the Transcribe handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetTranscribeHandler() service.Registerable { return c.transcribeHandler }

// GetSupportHandler returns the Support handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetSupportHandler() service.Registerable { return c.supportHandler }

// GetECRHandler returns the ECR handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetECRHandler() service.Registerable { return c.ecrHandler }

// GetECSHandler returns the ECS handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetECSHandler() service.Registerable { return c.ecsHandler }

// GetEFSHandler returns the EFS handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetEFSHandler() service.Registerable { return c.efsHandler }

// GetEKSHandler returns the EKS handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetEKSHandler() service.Registerable { return c.eksHandler }

// GetEMRHandler returns the EMR handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetEMRHandler() service.Registerable { return c.emrHandler }

// GetGlacierHandler returns the Glacier handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetGlacierHandler() service.Registerable { return c.glacierHandler }

// GetIoTAnalyticsHandler returns the IoT Analytics handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetIoTAnalyticsHandler() service.Registerable { return c.iotanalyticsHandler }

// GetIoTWirelessHandler returns the IoT Wireless handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetIoTWirelessHandler() service.Registerable { return c.iotwirelessHandler }

// GetKinesisAnalyticsHandler returns the Kinesis Analytics handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetKinesisAnalyticsHandler() service.Registerable { return c.kinesisanalyticsHandler }

// GetLakeFormationHandler returns the Lake Formation handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetLakeFormationHandler() service.Registerable { return c.lakeformationHandler }

// GetGlueHandler returns the Glue handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetGlueHandler() service.Registerable { return c.glueHandler }

// GetGuardDutyHandler returns the GuardDuty handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetGuardDutyHandler() service.Registerable { return c.guarddutyHandler }

// GetInspector2Handler returns the Inspector2 handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetInspector2Handler() service.Registerable { return c.inspector2Handler }

// GetKafkaHandler returns the Kafka handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetKafkaHandler() service.Registerable { return c.kafkaHandler }

// GetKinesisAnalyticsV2Handler returns the Kinesis Data Analytics v2 handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetKinesisAnalyticsV2Handler() service.Registerable {
	return c.kinesisanalyticsv2Handler
}

// GetManagedBlockchainHandler returns the Managed Blockchain handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetManagedBlockchainHandler() service.Registerable { return c.managedblockchainHandler }

// GetMediaConvertHandler returns the MediaConvert handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetMediaConvertHandler() service.Registerable { return c.mediaconvertHandler }

// GetMQHandler returns the Amazon MQ handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetMQHandler() service.Registerable { return c.mqHandler }

// GetMediaStoreHandler returns the MediaStore handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetMediaStoreHandler() service.Registerable { return c.mediastoreHandler }

// GetMediaStoreDataHandler returns the MediaStore Data handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetMediaStoreDataHandler() service.Registerable { return c.mediastoredataHandler }

// GetMemoryDBHandler returns the MemoryDB handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetMemoryDBHandler() service.Registerable { return c.memorydbHandler }

// GetOrganizationsHandler returns the Organizations handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetOrganizationsHandler() service.Registerable { return c.organizationsHandler }

// GetNeptuneHandler returns the Neptune handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetNeptuneHandler() service.Registerable { return c.neptuneHandler }

// GetMWAAHandler returns the MWAA handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetMWAAHandler() service.Registerable { return c.mwaaHandler }

// GetPinpointHandler returns the Pinpoint handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetPinpointHandler() service.Registerable { return c.pinpointHandler }

// GetPipesHandler returns the Pipes handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetPipesHandler() service.Registerable { return c.pipesHandler }

// GetRDSDataHandler returns the RDS Data handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetRDSDataHandler() service.Registerable { return c.rdsdataHandler }

// GetRAMHandler returns the RAM handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetRAMHandler() service.Registerable { return c.ramHandler }

// GetRedshiftDataHandler returns the Redshift Data handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetRedshiftDataHandler() service.Registerable { return c.redshiftdataHandler }

// GetSageMakerHandler returns the SageMaker handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetSageMakerHandler() service.Registerable { return c.sagemakerHandler }

// GetSageMakerRuntimeHandler returns the SageMaker Runtime handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetSageMakerRuntimeHandler() service.Registerable { return c.sagemakerRuntimeHandler }

// GetServiceDiscoveryHandler returns the Service Discovery handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetServiceDiscoveryHandler() service.Registerable { return c.servicediscoveryHandler }

// GetServerlessRepoHandler returns the Serverless Application Repository handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetServerlessRepoHandler() service.Registerable { return c.serverlessrepoHandler }

// GetShieldHandler returns the Shield handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetShieldHandler() service.Registerable { return c.shieldHandler }

// GetSsoAdminHandler returns the SSO Admin handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetSsoAdminHandler() service.Registerable { return c.ssoadminHandler }

// GetTextractHandler returns the Textract handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetTextractHandler() service.Registerable { return c.textractHandler }

// GetTimestreamWriteHandler returns the Timestream Write handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetTimestreamWriteHandler() service.Registerable { return c.timestreamwriteHandler }

// GetTimestreamQueryHandler returns the Timestream Query handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetTimestreamQueryHandler() service.Registerable { return c.timestreamqueryHandler }

// GetTransferHandler returns the Transfer handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetTransferHandler() service.Registerable { return c.transferHandler }

// GetVerifiedPermissionsHandler returns the Verified Permissions handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetVerifiedPermissionsHandler() service.Registerable {
	return c.verifiedPermissionsHandler
}

// GetWAFHandler returns the WAF Classic handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetWAFHandler() service.Registerable { return c.wafHandler }

// GetWafv2Handler returns the WAFv2 handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetWafv2Handler() service.Registerable { return c.wafv2Handler }

// GetXrayHandler returns the X-Ray handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetXrayHandler() service.Registerable { return c.xrayHandler }

// GetS3TablesHandler returns the S3 Tables handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetS3TablesHandler() service.Registerable { return c.s3tablesHandler }

// GetGrafanaHandler returns the Amazon Managed Grafana handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetGrafanaHandler() service.Registerable { return c.grafanaHandler }

// GetOutpostsHandler returns the AWS Outposts handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetOutpostsHandler() service.Registerable { return c.outpostsHandler }

// GetResilienceHubHandler returns the AWS Resilience Hub handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetResilienceHubHandler() service.Registerable { return c.resiliencehubHandler }

// GetDirectConnectHandler returns the AWS Direct Connect handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetDirectConnectHandler() service.Registerable { return c.directconnectHandler }

// GetMGNHandler returns the AWS Application Migration Service handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetMGNHandler() service.Registerable { return c.mgnHandler }

// GetNetworkManagerHandler returns the AWS Network Manager handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetNetworkManagerHandler() service.Registerable { return c.networkmanagerHandler }

// GetLightsailHandler returns the Amazon Lightsail handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetLightsailHandler() service.Registerable { return c.lightsailHandler }

// GetELBHandler returns the ELB handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetELBHandler() service.Registerable { return c.elbHandler }

// GetELBv2Handler returns the ELBv2 handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetELBv2Handler() service.Registerable { return c.elbv2Handler }

// GetEmrServerlessHandler returns the EMR Serverless handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetEmrServerlessHandler() service.Registerable { return c.emrserverlessHandler }

// GetIoTHandler returns the IoT handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetIoTHandler() service.Registerable { return c.iotHandler }

// GetAppSyncHandler returns the AppSync handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetAppSyncHandler() service.Registerable { return c.appSyncHandler }

// GetIoTDataPlaneHandler returns the IoT Data Plane handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetIoTDataPlaneHandler() service.Registerable { return c.iotDataPlaneHandler }

// GetAPIGatewayManagementAPIHandler returns the API Gateway Management API handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetAPIGatewayManagementAPIHandler() service.Registerable {
	return c.apiGatewayMgmtHandler
}

// GetAppConfigDataHandler returns the AppConfigData handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetAppConfigDataHandler() service.Registerable {
	return c.appConfigDataHandler
}

// GetAmplifyHandler returns the Amplify handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetAmplifyHandler() service.Registerable { return c.amplifyHandler }

// GetAutoscalingHandler returns the Autoscaling handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetAutoscalingHandler() service.Registerable { return c.autoscalingHandler }

// GetAPIGatewayV2Handler returns the API Gateway V2 handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetAPIGatewayV2Handler() service.Registerable { return c.apiGatewayV2Handler }

// GetAthenaHandler returns the Athena handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetAthenaHandler() service.Registerable { return c.athenaHandler }

// GetBackupHandler returns the Backup handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetBackupHandler() service.Registerable { return c.backupHandler }

// GetCloudTrailHandler returns the CloudTrail handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetCloudTrailHandler() service.Registerable { return c.cloudtrailHandler }

// GetAppConfigHandler returns the AppConfig handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetAppConfigHandler() service.Registerable { return c.appConfigHandler }

// GetApplicationAutoscalingHandler returns the Application Auto Scaling handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetApplicationAutoscalingHandler() service.Registerable {
	return c.applicationautoscalingHandler
}

// GetBatchHandler returns the Batch handler.
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetBatchHandler() service.Registerable { return c.batchHandler }

// GetBedrockHandler returns the Bedrock handler.
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetBedrockHandler() service.Registerable { return c.bedrockHandler }

// GetBedrockAgentsHandler returns the Bedrock Agents handler.
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetBedrockAgentsHandler() service.Registerable { return c.bedrockAgentsHandler }

// GetBedrockRuntimeHandler returns the Bedrock Runtime handler.
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetBedrockRuntimeHandler() service.Registerable { return c.bedrockruntimeHandler }

// GetCeHandler returns the Cost Explorer handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetCeHandler() service.Registerable { return c.ceHandler }

// GetCloudControlHandler returns the CloudControl handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn,nolintlint // architecturally required to return interface
func (c *CLI) GetCloudControlHandler() service.Registerable { return c.cloudcontrolHandler }

// GetCloudFrontHandler returns the CloudFront handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetCloudFrontHandler() service.Registerable { return c.cloudFrontHandler }

// GetCodeArtifactHandler returns the CodeArtifact handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetCodeArtifactHandler() service.Registerable { return c.codeArtifactHandler }

// GetCodeConnectionsHandler returns the CodeConnections handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetCodeConnectionsHandler() service.Registerable { return c.codeConnectionsHandler }

// GetCodeBuildHandler returns the CodeBuild handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetCodeBuildHandler() service.Registerable { return c.codebuildHandler }

// GetCodeCommitHandler returns the CodeCommit handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetCodeCommitHandler() service.Registerable { return c.codeCommitHandler }

// GetCodePipelineHandler returns the CodePipeline handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetCodePipelineHandler() service.Registerable { return c.codePipelineHandler }

// GetCodeDeployHandler returns the CodeDeploy handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetCodeDeployHandler() service.Registerable { return c.codeDeployHandler }

// GetDMSHandler returns the DMS handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetDMSHandler() service.Registerable { return c.dmsHandler }

// GetCodeStarConnectionsHandler returns the CodeStar Connections handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetCodeStarConnectionsHandler() service.Registerable {
	return c.codeStarConnectionsHandler
}

// GetDynamoDBStreamsHandler returns the DynamoDB Streams handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetDynamoDBStreamsHandler() service.Registerable {
	return c.dynamodbStreamsHandler
}

// GetElasticbeanstalkHandler returns the Elastic Beanstalk handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetElasticbeanstalkHandler() service.Registerable { return c.elasticbeanstalkHandler }

// GetDocDBHandler returns the DocDB handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetDocDBHandler() service.Registerable { return c.docdbHandler }

// GetFISHandler returns the FIS handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetFISHandler() service.Registerable { return c.fisHandler }

// GetIdentityStoreHandler returns the Identity Store handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetIdentityStoreHandler() service.Registerable { return c.identitystoreHandler }

// GetCognitoIDPHandler returns the Cognito IDP handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetCognitoIDPHandler() service.Registerable { return c.cognitoIDPHandler }

// GetCognitoIdentityHandler returns the Cognito Identity handler (dashboard.AWSSDKProvider).
//
//nolint:ireturn // architecturally required to return interface
func (c *CLI) GetCognitoIdentityHandler() service.Registerable { return c.cognitoIdentityHandler }

// GetFaultStore returns the chaos fault store (dashboard.AWSSDKProvider).
func (c *CLI) GetFaultStore() *chaos.FaultStore { return c.faultStore }

// GetPortAllocator returns the shared runtime port allocator.
func (c *CLI) GetPortAllocator() *portalloc.Allocator { return c.portAlloc }

// rootCLI is the top-level kong grammar. The server flags live in Serve
// (the default command); "health" is an explicit subcommand used as a
// Docker healthcheck from scratch containers.
type rootCLI struct {
	Health HealthCmd `cmd:"" help:"Check server health (for Docker healthcheck)."`
	Serve  CLI       `cmd:"" help:"Start the Gopherstack server."                 default:"withargs"`
}

// HealthCmd checks a running Gopherstack instance's health endpoint.
type HealthCmd struct {
	Port string `name:"port" env:"PORT" default:"8000" help:"Port of the running server to check."` //nolint:lll // config struct tags are intentionally verbose
}

var ErrHealthCheckFailed = errors.New("health check failed")

// Run executes the health check. Returns nil on success.
func (h *HealthCmd) Run() error {
	ctx, cancel := context.WithTimeout(context.Background(), healthCheckTimeout)
	defer cancel()

	client := &http.Client{}

	targetURL := &url.URL{
		Scheme: "http",
		Host:   "localhost:" + h.Port,
		Path:   "/_gopherstack/health",
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL.String(), nil)
	if err != nil {
		return fmt.Errorf("create health check request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrHealthCheckFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrHealthCheckFailed, resp.StatusCode)
	}

	fmt.Fprintln(os.Stdout, "ok")

	return nil
}

// Run parses CLI / environment-variable configuration and starts Gopherstack.
// It is called from main() and exits on error.
func Run() {
	var root rootCLI

	kctx := kong.Parse(
		&root,
		kong.Name("gopherstack"),
		kong.Description("In-memory AWS DynamoDB + S3 compatible server."),
	)

	// rootCtx is cancelled when SIGINT/SIGTERM arrives; all subsystems
	// (HTTP server, background workers, DNS server) derive their context
	// from this root so a single signal cleanly unwinds everything.
	rootCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	var err error
	switch kctx.Command() {
	case "health":
		err = root.Health.Run()
	default:
		// Opt-in pprof server for local profiling / PGO data collection.
		// No-op unless GOPHERSTACK_PPROF_ADDR is set; see startPprofServer.
		startPprofServer(buildLogger(root.Serve.LogLevel))
		err = run(rootCtx, root.Serve)
	}

	cancel() // release signal-handler goroutine resources

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// applyExplicitOverrides re-applies values from original (parsed from flags/env)
// back onto loaded (merged from persisted config) so that explicit CLI/env settings
// take precedence over persisted values.
// Precedence: defaults < persisted config < env/CLI.
//
// Standard AWS environment variables AWS_DEFAULT_REGION and AWS_REGION are also
// honoured as aliases for the region setting, matching LocalStack and awslocal behaviour.
func applyExplicitOverrides(original, loaded CLI) CLI {
	const defaultAccountID = "000000000000"
	if original.Region != "" && original.Region != defaultRegion {
		loaded.Region = original.Region
	}

	// Also accept the standard AWS SDK region environment variables so that
	// tools using `AWS_DEFAULT_REGION` or `AWS_REGION` work without remapping.
	// AWS_DEFAULT_REGION takes the highest precedence, then AWS_REGION,
	// then the REGION env var / --region flag (already applied above via original.Region).
	if r := os.Getenv("AWS_REGION"); r != "" {
		loaded.Region = r
	}

	if r := os.Getenv("AWS_DEFAULT_REGION"); r != "" {
		loaded.Region = r
	}

	if original.Port != "" && original.Port != defaultPort {
		loaded.Port = original.Port
	}
	if original.AccountID != "" && original.AccountID != defaultAccountID {
		loaded.AccountID = original.AccountID
	}
	if original.AutoPurgeTTL != 0 {
		loaded.AutoPurgeTTL = original.AutoPurgeTTL
	}

	return loaded
}

// setupPortAllocator creates a port allocator from the given range.
// Returns nil when the range is invalid (allocator disabled).
func setupPortAllocator(
	ctx context.Context,
	log *slog.Logger,
	start, end int,
) *portalloc.Allocator {
	alloc, err := portalloc.New(start, end)
	if err != nil {
		log.WarnContext(ctx, "Port allocator disabled (invalid range)", "error", err)

		return nil
	}
	log.InfoContext(
		ctx,
		"Port allocator ready",
		"start",
		start,
		"end",
		end,
		"available",
		alloc.Available(),
	)

	return alloc
}

// reserveFixedServicePorts marks ports bound directly by services outside
// the shared PortAlloc pool as unavailable within that pool, so Acquire
// never hands the same port number to a different caller.
//
// AzureBlob's dedicated listener (services/azureblob) binds a fixed,
// protocol-conventional default port (10000, matching Azurite's own Blob
// service port) via a raw net.Listen call, not through PortAlloc -- and that
// default sits squarely inside PortRangeStart/PortRangeEnd's own default
// range (10000-10100). Without this reservation, PortAlloc has no way to
// know AzureBlob already holds 10000 and could hand it to an unrelated
// caller (e.g. an ElastiCache instance), which would only surface later as
// a confusing address-in-use failure when that caller tries to actually
// bind it. See AZURE.md section 4 for the full rationale.
//
// A failed reservation is logged, not fatal: AzureBlob's own StartWorker
// bind is still synchronous and fails fast on a genuine conflict (see
// handler.go), so the worst outcome here is losing this early-warning
// cross-service protection, not an unrecoverable startup failure.
func reserveFixedServicePorts(ctx context.Context, log *slog.Logger, alloc *portalloc.Allocator, cli CLI) {
	if alloc == nil {
		return
	}

	if err := alloc.Reserve(cli.AzureBlob.Port, "azureblob"); err != nil {
		log.WarnContext(ctx, "failed to reserve AzureBlob's fixed port in the shared pool",
			"port", cli.AzureBlob.Port, "error", err)
	}

	// AzureQueue's dedicated listener (services/azurequeue) binds its own
	// fixed, protocol-conventional default port (10001, matching Azurite's
	// own Queue service port) the same way AzureBlob does above -- see that
	// call's comment and AZURE.md section 4 for the full rationale. It sits
	// in the same PortRangeStart/PortRangeEnd default range, so it needs the
	// same reservation.
	if err := alloc.Reserve(cli.AzureQueue.Port, "azurequeue"); err != nil {
		log.WarnContext(ctx, "failed to reserve AzureQueue's fixed port in the shared pool",
			"port", cli.AzureQueue.Port, "error", err)
	}

	// AzureTable's dedicated listener (services/azuretable) binds its own
	// fixed, protocol-conventional default port (10002, matching Azurite's
	// own Table service port) the same way AzureBlob/AzureQueue do above --
	// see those calls' comments and AZURE.md section 4 for the full
	// rationale. It sits in the same PortRangeStart/PortRangeEnd default
	// range, so it needs the same reservation.
	if err := alloc.Reserve(cli.AzureTable.Port, "azuretable"); err != nil {
		log.WarnContext(ctx, "failed to reserve AzureTable's fixed port in the shared pool",
			"port", cli.AzureTable.Port, "error", err)
	}

	// CosmosDB's dedicated listener (services/cosmosdb) binds its own fixed,
	// protocol-conventional default port -- but unlike AzureBlob/AzureQueue/
	// AzureTable above, that default (8081, the real Cosmos DB Local
	// Emulator's own published port) sits OUTSIDE PortRangeStart/
	// PortRangeEnd's own default range (10000-10100), exactly mirroring
	// services/iot's MQTT broker default (1883). This Reserve call is
	// therefore a no-op against the default range -- but the range is
	// user-configurable (--port-range-start/--port-range-end), so a custom
	// range that happens to include 8081 (e.g. 8000-8100) still needs this
	// reservation to keep PortAlloc from handing 8081 to an unrelated
	// caller. See AZURE.md section 4, services/cosmosdb/settings.go's
	// DefaultPort doc comment, and cli_cosmosdb_port_reservation_test.go's
	// table (which -- unlike AzureBlob/AzureQueue/AzureTable's tables --
	// covers both the outside-range default case AND a custom in-range
	// case, since only CosmosDB's default port has this inverted default
	// behavior).
	if err := alloc.Reserve(cli.CosmosDB.Port, "cosmosdb"); err != nil {
		log.WarnContext(ctx, "failed to reserve CosmosDB's fixed port in the shared pool",
			"port", cli.CosmosDB.Port, "error", err)
	}

	// AzureServiceBus's dedicated listener (services/azureservicebus) binds
	// its own fixed port (10003, the next available slot in gopherstack's own
	// numbering convention after AzureTable's 10002 -- there is no Azurite
	// Service Bus emulator to mirror) the same way AzureBlob/AzureQueue/
	// AzureTable do above -- see those calls' comments and AZURE.md section 9
	// (M5) for the full rationale. It sits in the same
	// PortRangeStart/PortRangeEnd default range, so it needs the same
	// reservation.
	if err := alloc.Reserve(cli.AzureServiceBus.Port, "azureservicebus"); err != nil {
		log.WarnContext(ctx, "failed to reserve AzureServiceBus's fixed port in the shared pool",
			"port", cli.AzureServiceBus.Port, "error", err)
	}
}

// setupPortAllocatorWithReservations builds the shared port allocator and
// reserves any fixed ports services bind directly (see
// reserveFixedServicePorts) before anything else can Acquire from it.
// Extracted from run() to keep both steps as a single statement there.
func setupPortAllocatorWithReservations(ctx context.Context, log *slog.Logger, cli CLI) *portalloc.Allocator {
	alloc := setupPortAllocator(ctx, log, cli.PortRangeStart, cli.PortRangeEnd)
	reserveFixedServicePorts(ctx, log, alloc, cli)

	return alloc
}

// run starts the server with the given CLI configuration.
// It is separated from Run so it can be exercised in tests without [os.Exit].
func run(ctx context.Context, cli CLI) error {
	log := buildLogger(cli.LogLevel)
	ctx = logger.Save(ctx, log)

	// Take a snapshot of the CLI values as parsed from flags/env so that explicit
	// CLI/env settings take precedence over any persisted config.
	original := cli

	if cli.Persist {
		if err := cli.LoadConfig(); err != nil {
			log.WarnContext(ctx, "Failed to load config", "error", err)
		}
	}

	cli = applyExplicitOverrides(original, cli)
	cli.GetGlobalConfig().Update(
		cli.AccountID, cli.Region, cli.LatencyMs,
		cli.JanitorTimeout, cli.EnforceIAM, cli.AutoPurgeTTL,
	)

	// --- Port allocator ---
	cli.portAlloc = setupPortAllocatorWithReservations(ctx, log, cli)

	// --- Embedded DNS server ---
	var dnsSrv *gopherDNS.Server
	if cli.DNSListenAddr != "" {
		dnsSrv = startEmbeddedDNS(ctx, cli.DNSListenAddr, cli.DNSResolveIP)
	}

	inMemMux := http.NewServeMux()
	inMemClient := &dashboard.InMemClient{Handler: inMemMux}

	awsCfgVal, err := buildInternalAWSConfig(ctx, cli.Region, inMemClient)
	if err != nil {
		log.ErrorContext(ctx, "Failed to load AWS config", "error", err)

		return err
	}

	initializeClients(&cli, awsCfgVal)

	janitorCtx, janitorCancel := context.WithCancel(ctx)
	defer janitorCancel() // also passed to shutdownBackends so janitors stop before backends are torn down

	// --- Persistence ---
	persistManager, err := initPersistenceManager(ctx, &cli)
	if err != nil {
		return err
	}

	if cli.Persist {
		defer persistManager.SaveAll(ctx)
	}

	appCtx := &service.AppContext{
		Logger:         log,
		Config:         &cli,
		JanitorCtx:     janitorCtx,
		JanitorTimeout: cli.JanitorTimeout,
		PortAlloc:      cli.portAlloc,
	}

	// Create the fault store before initialising services so the dashboard can
	// receive it via cli.GetFaultStore() during its Init() call.
	cli.faultStore = chaos.NewFaultStore()

	services, err := initializeServices(appCtx)
	if err != nil {
		return err
	}

	setupPersistence(ctx, persistManager, services, cli.Persist, cli.Region)

	if dnsSrv != nil {
		wireDNSRegistrars(&cli, dnsSrv)
	}

	e := buildEchoServer(ctx, log, persistManager, services, cli)

	if setupErr := setupChaosAndRegistry(e, log, &cli, services); setupErr != nil {
		return setupErr
	}

	startBackgroundWorkers(janitorCtx, services)

	// Start automatic purge background worker. It dynamically checks for TTL updates from the config.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.ErrorContext(ctx, "purge worker: panic recovered",
					"panic", fmt.Sprintf("%v", r))
			}
		}()
		startPurgeWorker(janitorCtx, cli.globalConfig, services)
	}()

	inMemMux.Handle("/", e)

	if cli.Demo {
		loadDemoData(ctx, &cli)
	}

	runInitHooks(ctx, &cli, log)
	createS3InitBuckets(ctx, &cli, log)
	defer shutdownBackends(janitorCancel, cli.lambdaHandler, services)

	return startServer(ctx, cli.Port, e, tlsConfigFromCLI(&cli))
}

// tlsSettings carries the resolved TLS configuration for the listener.
type tlsSettings struct {
	// certFile / keyFile point to PEM files; when both empty (and enabled), a
	// self-signed certificate is generated in-memory on startup.
	certFile string
	keyFile  string
	// enabled is true when the server should serve HTTPS.
	enabled bool
}

// tlsConfigFromCLI derives the TLS listener settings from CLI flags. TLS is
// enabled when --tls is set or when an explicit cert/key pair is supplied.
func tlsConfigFromCLI(cli *CLI) tlsSettings {
	enabled := cli.TLS || (cli.TLSCertFile != "" && cli.TLSKeyFile != "")

	return tlsSettings{
		enabled:  enabled,
		certFile: cli.TLSCertFile,
		keyFile:  cli.TLSKeyFile,
	}
}

// runInitHooks runs init scripts after all services are ready, if any are configured.
func runInitHooks(ctx context.Context, cli *CLI, log *slog.Logger) {
	if len(cli.InitScripts) == 0 {
		return
	}

	runner := inithooks.New(cli.InitScripts, cli.InitScriptTimeout, log)
	runner.Run(ctx)
}

// createS3InitBuckets creates S3 buckets listed in cli.S3InitBuckets on startup.
// Bucket names may be passed individually (--s3-bucket name) or as a comma-separated
// list (S3_BUCKETS=a,b,c), matching the awslocal convention of `awslocal s3 mb s3://name`.
// Errors are logged but do not abort server startup.
func createS3InitBuckets(ctx context.Context, cli *CLI, log *slog.Logger) {
	if len(cli.S3InitBuckets) == 0 {
		return
	}

	// Flatten comma-separated entries that may come from the S3_BUCKETS env var.
	var buckets []string

	for _, entry := range cli.S3InitBuckets {
		for name := range strings.SplitSeq(entry, ",") {
			if trimmed := strings.TrimSpace(name); trimmed != "" {
				buckets = append(buckets, trimmed)
			}
		}
	}

	for _, bucket := range buckets {
		if _, err := cli.s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(bucket),
		}); err != nil {
			log.WarnContext(ctx, "Failed to create init S3 bucket", "bucket", bucket, "error", err)
		} else {
			log.InfoContext(ctx, "Created S3 bucket on startup", "bucket", bucket)
		}
	}
}

// buildInternalAWSConfig constructs an [aws.Config] for the server's own SDK clients.
// AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY are used when set, so that tooling that
// configures these standard env vars (e.g. awslocal) works without credential remapping.
// The server never validates incoming credentials, so the exact values do not matter.
func buildInternalAWSConfig(
	ctx context.Context,
	region string,
	httpClient awscfg.HTTPClient,
) (aws.Config, error) {
	keyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secret := os.Getenv("AWS_SECRET_ACCESS_KEY")

	if keyID == "" {
		keyID = "dummy"
	}

	if secret == "" {
		secret = "dummy"
	}

	return awscfg.LoadDefaultConfig(
		ctx,
		awscfg.WithRegion(region),
		awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(keyID, secret, "")),
		awscfg.WithHTTPClient(httpClient),
	)
}

// lambdaCloseFn returns a cleanup function that shuts down the Lambda backend's
// function URL servers and runtime API servers, or nil if the handler is not a Lambda backend.
func lambdaCloseFn(lambdaReg service.Registerable) func() {
	lambdaH, lambdaOk := lambdaReg.(*lambdabackend.Handler)
	if !lambdaOk {
		return nil
	}

	lambdaBk, bkOk := lambdaH.Backend.(*lambdabackend.InMemoryBackend)
	if !bkOk {
		return nil
	}

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		lambdaBk.Close(ctx)
	}
}

// shutdownBackends cancels background workers, shuts down the Lambda backend,
// and then shuts down every service that implements service.Shutdowner. It is
// called via defer after the HTTP server has stopped accepting requests.
// janitorCancel is called first so that janitor goroutines stop before the
// backends they access are torn down.
func shutdownBackends(
	janitorCancel context.CancelFunc,
	lambdaHandler service.Registerable,
	services []service.Registerable,
) {
	// Stop janitor workers before closing backends they may still be accessing.
	janitorCancel()

	if closeFn := lambdaCloseFn(lambdaHandler); closeFn != nil {
		closeFn()
	}

	shutCtx, shutCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutCancel()

	shutdownServices(shutCtx, services)
}

// wireDNSRegistrars connects DNS-aware backends to the embedded DNS server.
func wireDNSRegistrars(cli *CLI, dnsSrv *gopherDNS.Server) {
	wireLambdaDNS(cli.lambdaHandler, dnsSrv)
	wireRoute53DNS(cli.route53Handler, dnsSrv)
	wireRDSDNS(cli.rdsHandler, dnsSrv)
	wireRedshiftDNS(cli.redshiftHandler, dnsSrv)
	wireElasticsearchDNS(cli.elasticsearchHandler, dnsSrv)
	wireOpenSearchDNS(cli.openSearchHandler, dnsSrv)
	wireElastiCacheDNS(cli.elasticacheHandler, dnsSrv)
	wireEC2DNS(cli.ec2Handler, dnsSrv)
}

// buildEchoServer creates and configures the Echo HTTP server.
func buildEchoServer(
	ctx context.Context,
	log *slog.Logger,
	persistManager *persistence.Manager,
	services []service.Registerable,
	cli CLI,
) *echo.Echo {
	e := echo.New()
	e.Use(panicRecoveryMiddleware())
	e.Use(httputils.RequestIDMiddleware())
	e.Use(logger.APIConsoleMiddleware())
	e.Use(telemetry.MemoryStatsMiddleware)
	e.Pre(logger.EchoMiddleware(log))
	e.Pre(awsMetaMiddleware(cli.Region, cli.AccountID))

	// Optional, opt-in SigV4 signature validation. Off by default so existing
	// clients (which sign with dummy creds) are not rejected.
	if cli.ValidateSigV4 {
		log.InfoContext(ctx, "SigV4 request-signature validation ENABLED")
		e.Use(httputils.NewSigV4Validator(cli.SigV4Secret).EchoMiddleware())
	}

	e.HTTPErrorHandler = buildHTTPErrorHandler()
	e.GET("/favicon.ico", func(c *echo.Context) error {
		return c.Redirect(http.StatusFound, "/dashboard/static/favicon.png")
	})
	e.GET("/_gopherstack/health", buildHealthHandler(services))
	e.GET("/_localstack/health", buildLocalstackHealthHandler(services))
	e.GET("/_aws/health", buildLocalstackHealthHandler(services))
	e.GET("/_localstack/init", buildLocalstackInitHandler())
	e.GET("/_localstack/init/ready", buildLocalstackInitHandler())
	e.GET("/_localstack/info", buildLocalstackInfoHandler())
	e.POST("/_gopherstack/reset", buildResetHandler(services))
	e.POST("/_gopherstack/snapshot", buildSnapshotHandler(persistManager))
	e.POST("/_gopherstack/load", buildLoadHandler(persistManager))

	registerWebsiteRoutes(e, services)

	if cli.Persist {
		e.Use(persistenceMiddleware(persistManager, services))
	}

	return e
}

// buildHTTPErrorHandler returns an Echo error handler that logs 5xx errors via
// slog and writes the standard JSON error response.
//
// Status comes from echo.StatusCode(err), not a type-assert to *echo.HTTPError:
// echo v5's own ErrNotFound/ErrMethodNotAllowed/etc are a different unexported
// *httpError type that only satisfies the HTTPStatusCoder interface, so the old
// assert missed them and reported every unmatched route (e.g. GET /favicon.ico)
// as a 500 instead of its real 404.
func buildHTTPErrorHandler() func(*echo.Context, error) {
	return func(c *echo.Context, err error) {
		code := echo.StatusCode(err)
		if code == 0 {
			code = http.StatusInternalServerError
		}

		message := err.Error()

		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) {
			message = httpErr.Message
		}

		if code >= http.StatusInternalServerError {
			logger.Load(c.Request().Context()).ErrorContext(
				c.Request().Context(), "HTTP error",
				"status", code,
				"error", message,
				"path", c.Request().URL.Path,
				"method", c.Request().Method,
			)
		}

		if resp, _ := echo.UnwrapResponse(c.Response()); resp == nil || !resp.Committed {
			_ = c.JSON(code, map[string]any{keyMessageField: message})
		}
	}
}

// buildHealthHandler returns the /_gopherstack/health handler. It reports service
// names and runtime memory/goroutine stats.
func buildHealthHandler(services []service.Registerable) echo.HandlerFunc {
	return func(c *echo.Context) error {
		names := make([]string, 0, len(services))
		for _, svc := range services {
			names = append(names, svc.Name())
		}

		sort.Strings(names)

		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)

		return c.JSON(http.StatusOK, healthResponse{
			Status:     "ok",
			Version:    version.Get(),
			Services:   names,
			Goroutines: runtime.NumGoroutine(),
			HeapAllocB: ms.HeapAlloc,
			HeapInuseB: ms.HeapInuse,
			NumGC:      ms.NumGC,
		})
	}
}

// buildResetHandler returns the /_gopherstack/reset handler that clears all
// in-memory state for every service that supports it.
func buildResetHandler(services []service.Registerable) echo.HandlerFunc {
	return func(c *echo.Context) error {
		reset := 0
		reqService := c.QueryParam("service")

		for _, svc := range services {
			if r, ok := svc.(service.Resettable); ok {
				if reqService != "" && !strings.EqualFold(reqService, svc.Name()) {
					continue
				}
				r.Reset()
				reset++
			}
		}

		return c.JSON(http.StatusOK, map[string]any{
			"status":        "ok",
			"reset":         reset,
			keyMessageField: fmt.Sprintf("reset %d service(s)", reset),
		})
	}
}

// snapshotBundle is the JSON envelope returned by POST /_gopherstack/snapshot
// and accepted by POST /_gopherstack/load.
//
// Each value in Services is a raw JSON blob produced by the service's
// Snapshot() method, embedded verbatim so that round-trips are lossless and the
// payload stays human-readable without an extra base64 layer.
type snapshotBundle struct {
	// Services maps each registered service name to its raw JSON snapshot.
	Services map[string]json.RawMessage `json:"services"`
	// Format is a fixed version tag; currently "gopherstack-snapshot/v1".
	Format string `json:"format"`
}

// snapshotResponse is the JSON body returned by POST /_gopherstack/snapshot.
type snapshotResponse struct {
	snapshotBundle
	Status string `json:"status"`
	// Exported is the number of services whose snapshots were included.
	Exported int `json:"exported"`
}

// loadResponse is the JSON body returned by POST /_gopherstack/load.
type loadResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	// Loaded is the number of services successfully restored.
	Loaded int `json:"loaded"`
}

// snapshotBundleFormat is the format identifier embedded in every snapshot bundle.
const snapshotBundleFormat = "gopherstack-snapshot/v1"

// buildSnapshotHandler returns the POST /_gopherstack/snapshot handler.
//
// It calls ExportAll on the persistence manager to collect the current
// in-memory state of every registered service, wraps the snapshots in a
// snapshotBundle envelope, and returns it as the response body. The caller
// can store the blob and later replay it via POST /_gopherstack/load to
// restore state (Cloud-Pods style).
//
// The handler does not write to the underlying Store; if the caller also
// wants on-disk persistence they should use --persist/--data-dir together
// with the debounced auto-snapshot that fires after every mutation.
func buildSnapshotHandler(m *persistence.Manager) echo.HandlerFunc {
	return func(c *echo.Context) error {
		raw := m.ExportAll()

		services := make(map[string]json.RawMessage, len(raw))
		for name, data := range raw {
			services[name] = json.RawMessage(data)
		}

		resp := snapshotResponse{
			snapshotBundle: snapshotBundle{
				Format:   snapshotBundleFormat,
				Services: services,
			},
			Exported: len(services),
			Status:   "ok",
		}

		return c.JSON(http.StatusOK, resp)
	}
}

// buildLoadHandler returns the POST /_gopherstack/load handler.
//
// It reads a snapshotBundle from the request body and restores each service's
// state by calling ImportAll on the persistence manager. Services present in
// the bundle but not registered with the manager are warned and skipped.
// Returns 400 for malformed input and 500 if any restore fails.
func buildLoadHandler(m *persistence.Manager) echo.HandlerFunc {
	return func(c *echo.Context) error {
		var bundle snapshotBundle

		if err := json.NewDecoder(c.Request().Body).Decode(&bundle); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest,
				fmt.Sprintf("invalid snapshot bundle: %s", err.Error()))
		}

		snapshots := make(map[string][]byte, len(bundle.Services))
		for name, raw := range bundle.Services {
			snapshots[name] = []byte(raw)
		}

		ctx := c.Request().Context()

		if err := m.ImportAll(ctx, snapshots); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError,
				fmt.Sprintf("restore failed: %s", err.Error()))
		}

		resp := loadResponse{
			Status:  "ok",
			Loaded:  len(snapshots),
			Message: fmt.Sprintf("restored %d service(s)", len(snapshots)),
		}

		return c.JSON(http.StatusOK, resp)
	}
}

// registerWebsiteRoutes registers the S3 website-serving routes on e.
// It finds the S3 handler from the registered services list.
func registerWebsiteRoutes(e *echo.Echo, services []service.Registerable) {
	for _, svc := range services {
		s3H, ok := svc.(*s3backend.S3Handler)
		if !ok {
			continue
		}

		e.GET("/_gopherstack/website/:bucket/*", s3H.ServeWebsite)
		e.GET("/_gopherstack/website/:bucket", func(c *echo.Context) error {
			bucket := c.Param("bucket")

			return c.Redirect(http.StatusMovedPermanently, "/_gopherstack/website/"+bucket+"/")
		})

		break
	}
}

// initializeClients configures the AWS SDK clients for DynamoDB, S3, SSM, and STS.
func initializeClients(cli *CLI, awsCfg aws.Config) {
	cli.ddbClient = dynamodb.NewFromConfig(
		awsCfg,
		func(o *dynamodb.Options) {
			o.BaseEndpoint = aws.String("http://local")
		},
	)
	cli.s3Client = s3.NewFromConfig(
		awsCfg,
		func(o *s3.Options) {
			o.UsePathStyle = true
			o.BaseEndpoint = aws.String("http://local")
		},
	)
	cli.ssmClient = ssmsdk.NewFromConfig(
		awsCfg,
		func(o *ssmsdk.Options) {
			o.BaseEndpoint = aws.String("http://local")
		},
	)
	cli.stsClient = stssdk.NewFromConfig(
		awsCfg,
		func(o *stssdk.Options) {
			o.BaseEndpoint = aws.String("http://local")
		},
	)
	cli.sqsClient = sqssdk.NewFromConfig(
		awsCfg,
		func(o *sqssdk.Options) {
			o.BaseEndpoint = aws.String("http://local")
		},
	)
	cli.snsClient = sns.NewFromConfig(
		awsCfg,
		func(o *sns.Options) {
			o.BaseEndpoint = aws.String("http://local")
		},
	)
	cli.iamClient = iam.NewFromConfig(
		awsCfg,
		func(o *iam.Options) {
			o.BaseEndpoint = aws.String("http://local")
		},
	)
	cli.kmsClient = kms.NewFromConfig(
		awsCfg,
		func(o *kms.Options) {
			o.BaseEndpoint = aws.String("http://local")
		},
	)
	cli.secretsManagerClient = secretsmanager.NewFromConfig(
		awsCfg,
		func(o *secretsmanager.Options) {
			o.BaseEndpoint = aws.String("http://local")
		},
	)
	cli.ecrClient = ecr.NewFromConfig(
		awsCfg,
		func(o *ecr.Options) {
			o.BaseEndpoint = aws.String("http://local")
		},
	)
	cli.appSyncSdkClient = appsyncsdksvc.NewFromConfig(
		awsCfg,
		func(o *appsyncsdksvc.Options) {
			o.BaseEndpoint = aws.String("http://local")
		},
	)
	cli.amplifyClient = amplifysdk.NewFromConfig(
		awsCfg,
		func(o *amplifysdk.Options) {
			o.BaseEndpoint = aws.String("http://local")
		},
	)
	cli.ecsClient = ecs.NewFromConfig(
		awsCfg,
		func(o *ecs.Options) {
			o.BaseEndpoint = aws.String("http://local")
		},
	)
	cli.eksClient = ekssdk.NewFromConfig(
		awsCfg,
		func(o *ekssdk.Options) {
			o.BaseEndpoint = aws.String("http://local")
		},
	)
	initializeIoTAndCodeClients(cli, awsCfg)
}

// initializeIoTAndCodeClients configures IoT, CodeDeploy and CodePipeline SDK clients.
func initializeIoTAndCodeClients(cli *CLI, awsCfg aws.Config) {
	cli.iotClient = iotsdk.NewFromConfig(
		awsCfg,
		func(o *iotsdk.Options) {
			o.BaseEndpoint = aws.String("http://local")
		},
	)
	cli.codeDeployClient = codedeploysdk.NewFromConfig(
		awsCfg,
		func(o *codedeploysdk.Options) {
			o.BaseEndpoint = aws.String("http://local")
		},
	)
	cli.codePipelineSDKClient = codepipelinesdk.NewFromConfig(
		awsCfg,
		func(o *codepipelinesdk.Options) {
			o.BaseEndpoint = aws.String("http://local")
		},
	)
}

// serviceByName builds a lookup map from service Name() to the service instance.
func serviceByName(services []service.Registerable) map[string]service.Registerable {
	m := make(map[string]service.Registerable, len(services))
	for _, svc := range services {
		m[svc.Name()] = svc
	}

	return m
}

// storeCLIHandlers assigns initialized service handlers to the CLI fields using name-based lookup.
func storeCLIHandlers(cli *CLI, services []service.Registerable) {
	byName := serviceByName(services)

	cli.ddbHandler = byName["DynamoDB"]
	cli.s3Handler = byName["S3"]
	cli.ssmHandler = byName["SSM"]
	cli.iamHandler = byName["IAM"]
	cli.stsHandler = byName["STS"]
	cli.snsHandler = byName["SNS"]
	cli.sqsHandler = byName["SQS"]
	cli.kmsHandler = byName["KMS"]
	cli.secretsManagerHandler = byName["SecretsManager"]
	cli.lambdaHandler = byName["Lambda"]
	cli.eventBridgeHandler = byName["EventBridge"]
	cli.apiGatewayHandler = byName["APIGateway"]
	cli.cloudWatchLogsHandler = byName["CloudWatchLogs"]
	cli.stepFunctionsHandler = byName["StepFunctions"]
	cli.cloudWatchHandler = byName["CloudWatch"]
	cli.kinesisHandler = byName["Kinesis"]
	cli.elasticacheHandler = byName["ElastiCache"]
	cli.route53Handler = byName["Route53"]
	cli.sesHandler = byName["SES"]
	cli.sesv2Handler = byName["SESv2"]
	cli.ec2Handler = byName["EC2"]
	cli.elasticsearchHandler = byName["Elasticsearch"]
	cli.openSearchHandler = byName["OpenSearch"]
	cli.acmHandler = byName["ACM"]
	cli.acmpcaHandler = byName["ACMPCA"]
	cli.redshiftHandler = byName["Redshift"]
	cli.redshiftServerlessHandler = byName["RedshiftServerless"]
	cli.awsconfigHandler = byName["AWSConfig"]
	cli.s3controlHandler = byName["S3Control"]
	cli.resourcegroupsHandler = byName["ResourceGroups"]
	cli.resourcegroupstaggingHandler = byName["ResourceGroupsTaggingAPI"]
	cli.swfHandler = byName["SWF"]
	cli.firehoseHandler = byName["Firehose"]
	cli.networkmonitorHandler = byName["NetworkMonitor"]
	cli.schedulerHandler = byName["Scheduler"]
	cli.route53resolverHandler = byName["Route53Resolver"]
	cli.rdsHandler = byName["RDS"]
	cli.transcribeHandler = byName["Transcribe"]
	cli.supportHandler = byName["Support"]
	cli.appSyncHandler = byName["AppSync"]

	storeCLIRecentHandlers(cli, byName)
}

// storeCLIRecentHandlers assigns recently-added service handlers to the CLI fields.
func storeCLIRecentHandlers(cli *CLI, byName map[string]service.Registerable) {
	cli.iotDataPlaneHandler = byName["IoTDataPlane"]
	cli.apiGatewayMgmtHandler = byName["APIGatewayManagementAPI"]

	storeAdditionalCLIHandlers(cli, byName)
}

// storeAdditionalCLIHandlers stores recently-added service handlers into the CLI struct.
func storeAdditionalCLIHandlers(cli *CLI, byName map[string]service.Registerable) {
	cli.appConfigDataHandler = byName["AppConfigData"]
	cli.amplifyHandler = byName["Amplify"]
	cli.autoscalingHandler = byName["Autoscaling"]
	cli.apiGatewayV2Handler = byName["APIGatewayV2"]
	storeCLIExtendedHandlers(cli, byName)
}

// storeCLIExtendedHandlers assigns handlers for services added after the initial set.
func storeCLIExtendedHandlers(cli *CLI, byName map[string]service.Registerable) {
	cli.athenaHandler = byName["Athena"]
	cli.appConfigHandler = byName["AppConfig"]
	cli.applicationautoscalingHandler = byName["ApplicationAutoscaling"]
	cli.batchHandler = byName["Batch"]
	cli.bedrockHandler = byName["Bedrock"]
	cli.bedrockAgentsHandler = byName["BedrockAgents"]
	cli.bedrockruntimeHandler = byName["BedrockRuntime"]
	cli.ecrHandler = byName["ECR"]
	cli.ecsHandler = byName["ECS"]
	cli.iotHandler = byName["IoT"]
	cli.cognitoIDPHandler = byName["CognitoIDP"]
	cli.cognitoIdentityHandler = byName["CognitoIdentity"]
	cli.fisHandler = byName["FIS"]
	cli.identitystoreHandler = byName["IdentityStore"]
	cli.backupHandler = byName["Backup"]
	cli.cloudtrailHandler = byName["CloudTrail"]
	cli.ceHandler = byName["Ce"]
	cli.cloudcontrolHandler = byName["CloudControl"]
	cli.cloudFrontHandler = byName["CloudFront"]
	cli.codeArtifactHandler = byName["CodeArtifact"]
	cli.codeConnectionsHandler = byName["CodeConnections"]
	cli.codebuildHandler = byName["CodeBuild"]
	cli.codeCommitHandler = byName["CodeCommit"]
	cli.codePipelineHandler = byName["CodePipeline"]
	cli.codeDeployHandler = byName["CodeDeploy"]
	cli.dmsHandler = byName["DMS"]
	cli.codeStarConnectionsHandler = byName["CodeStarConnections"]
	cli.dynamodbStreamsHandler = byName["DynamoDBStreams"]
	cli.elasticbeanstalkHandler = byName["Elasticbeanstalk"]
	cli.efsHandler = byName["EFS"]
	cli.eksHandler = byName["EKS"]
	cli.elbHandler = byName["ELB"]
	cli.elbv2Handler = byName["ELBv2"]
	cli.emrserverlessHandler = byName["EmrServerless"]
	cli.emrHandler = byName["EMR"]
	storeCLILatestHandlers(cli, byName)
}

// storeCLILatestHandlers assigns the newest service handlers to the CLI fields.
func storeCLILatestHandlers(cli *CLI, byName map[string]service.Registerable) {
	cli.glacierHandler = byName["Glacier"]
	cli.iotwirelessHandler = byName["IoTWireless"]
	cli.kinesisanalyticsHandler = byName["KinesisAnalytics"]
	cli.lakeformationHandler = byName["LakeFormation"]
	cli.glueHandler = byName["Glue"]
	cli.guarddutyHandler = byName["GuardDuty"]
	cli.inspector2Handler = byName["Inspector2"]
	cli.iotanalyticsHandler = byName["IoTAnalytics"]
	cli.kafkaHandler = byName["Kafka"]
	cli.kinesisanalyticsv2Handler = byName["KinesisAnalyticsV2"]
	cli.managedblockchainHandler = byName["ManagedBlockchain"]
	cli.mediaconvertHandler = byName["MediaConvert"]
	cli.mqHandler = byName["MQ"]
	cli.mediastoreHandler = byName["MediaStore"]
	cli.mediastoredataHandler = byName["MediaStoreData"]
	storeCLINewestHandlers(cli, byName)
}

// storeCLINewestHandlers assigns handlers for the most recently added services.
func storeCLINewestHandlers(cli *CLI, byName map[string]service.Registerable) {
	cli.memorydbHandler = byName["MemoryDB"]
	cli.organizationsHandler = byName["Organizations"]
	cli.mwaaHandler = byName["MWAA"]
	cli.neptuneHandler = byName["Neptune"]
	cli.docdbHandler = byName["DocDB"]
	cli.pinpointHandler = byName["Pinpoint"]
	cli.pipesHandler = byName["Pipes"]
	cli.rdsdataHandler = byName["RDSData"]
	cli.ramHandler = byName["RAM"]
	cli.redshiftdataHandler = byName["RedshiftData"]
	cli.sagemakerHandler = byName["SageMaker"]
	cli.sagemakerRuntimeHandler = byName["SageMakerRuntime"]
	cli.servicediscoveryHandler = byName["ServiceDiscovery"]
	cli.serverlessrepoHandler = byName["ServerlessRepo"]
	cli.shieldHandler = byName["Shield"]
	cli.ssoadminHandler = byName["SsoAdmin"]
	cli.textractHandler = byName["Textract"]
	cli.comprehendHandler = byName["Comprehend"]
	cli.timestreamwriteHandler = byName["TimestreamWrite"]
	cli.timestreamqueryHandler = byName["TimestreamQuery"]
	cli.transferHandler = byName["Transfer"]
	cli.verifiedPermissionsHandler = byName["VerifiedPermissions"]
	cli.wafHandler = byName["WAF"]
	cli.wafv2Handler = byName["Wafv2"]
	cli.xrayHandler = byName["Xray"]
	cli.s3tablesHandler = byName["S3tables"]
	cli.grafanaHandler = byName["Grafana"]
	cli.outpostsHandler = byName["Outposts"]
	cli.resiliencehubHandler = byName["ResilienceHub"]
	cli.directconnectHandler = byName["DirectConnect"]
	cli.mgnHandler = byName["MGN"]
	cli.networkmanagerHandler = byName["NetworkManager"]
	cli.lightsailHandler = byName["Lightsail"]
}

// initializeServices initializes all service providers, wires the
// cross-service integrations that need another service's handler, then
// registers CloudFormation and the dashboard last. The three phases mirror
// what each step depends on: independent providers, providers needing
// another service's already-initialized handler, and providers needing the
// full service registry (CloudFormation, then the dashboard).
func initializeServices(appCtx *service.AppContext) ([]service.Registerable, error) {
	services, err := initIndependentServices(appCtx)
	if err != nil {
		return nil, err
	}

	// Store handlers in CLI so dashboard and CloudFormation can access them.
	if cli, ok := appCtx.Config.(*CLI); ok {
		storeCLIHandlers(cli, services)
	}

	// Build name-based lookup for cross-service wiring.
	byName := serviceByName(services)

	wireCrossServiceDependencies(appCtx, services, byName)

	services, err = registerCloudFormationAndDashboard(appCtx, services, byName)
	if err != nil {
		return nil, err
	}

	// Record the service count so the metrics dashboard (and Prometheus scrape)
	// can surface it alongside the health endpoint's "services" field.
	telemetry.SetServiceCount(len(services))

	// The router sorts services by MatchPriority() at startup, so registration order
	// does not affect routing correctness.
	return services, nil
}

// initIndependentServices constructs every service provider that has no
// dependency on another service's handler at init time.
func initIndependentServices(appCtx *service.AppContext) ([]service.Registerable, error) {
	providers := getServiceProviders()
	services := make([]service.Registerable, 0, len(providers))

	for _, provider := range providers {
		svc, err := provider.Init(appCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to init %s: %w", provider.Name(), err)
		}

		services = append(services, svc)
	}

	return services, nil
}

// wireCrossServiceDependencies wires every cross-service integration that
// needs another service's already-initialized handler. It dispatches, in
// the original registration order, to helpers grouped by what the
// dependencies are (messaging/eventing, Step Functions, SSM/Kinesis key and
// notification wiring, API Gateway, event-source pollers,
// compute/observability, CloudWatch Logs metric emitters, storage/secrets,
// AppSync/streams, Scheduler/Pipes, and governance) rather than by an
// arbitrary line-count split.
func wireCrossServiceDependencies(
	appCtx *service.AppContext,
	services []service.Registerable,
	byName map[string]service.Registerable,
) {
	wireMessagingAndEventingIntegrations(byName)
	wireStepFunctionsIntegrations(byName)
	wireParameterAndKeyIntegrations(byName)
	wireAPIGatewayIntegrations(byName)
	wireEventSourcePollers(byName)
	wireComputeAndObservabilityIntegrations(appCtx, byName)
	wireCWLogsMetricEmitters(byName)
	wireStorageAndSecretsIntegrations(byName)
	wireAppSyncAndStreamsIntegrations(byName)
	wireSchedulerAndPipesIntegrations(byName)
	wireGovernanceIntegrations(byName, services)
}

// wireMessagingAndEventingIntegrations wires SNS, SQS, EventBridge, and S3
// notification delivery.
func wireMessagingAndEventingIntegrations(byName map[string]service.Registerable) {
	// Wire SNS→SQS delivery: when SNS publishes a message, deliver it to SQS queues.
	wireSNSToSQS(byName["SNS"], byName["SQS"])

	// Wire SNS→Lambda and SNS→Firehose subscription delivery, plus the SQS sender
	// used for Lambda/Firehose subscription DLQ redelivery.
	wireSNSToLambdaFirehose(byName["SNS"], byName["Lambda"], byName["Firehose"], byName["SQS"])

	// Wire SQS → CloudWatch metric emission for NumberOfMessagesSent/Received/Deleted.
	wireSQSMetrics(byName["SQS"], byName["CloudWatch"])

	// Wire EventBridge target fan-out: deliver events to Lambda, SQS, SNS, Kinesis,
	// Firehose, ECS, Step Functions, CloudWatch Logs, and API Destination targets.
	wireEventBridgeDelivery(
		byName["EventBridge"],
		byName["Lambda"],
		byName["SQS"],
		byName["SNS"],
		byName["Kinesis"],
		byName["Firehose"],
		byName["ECS"],
		byName["StepFunctions"],
		byName["CloudWatchLogs"],
	)

	// Wire S3 bucket notification delivery to SQS/SNS/Lambda targets.
	wireS3Notifications(
		byName["S3"],
		byName["SQS"],
		byName["SNS"],
		byName["Lambda"],
		byName["EventBridge"],
	)
}

// wireStepFunctionsIntegrations wires Step Functions' Lambda Task and
// direct service integrations.
func wireStepFunctionsIntegrations(byName map[string]service.Registerable) {
	// Wire Step Functions → Lambda Task integration.
	wireStepFunctionsLambda(byName["StepFunctions"], byName["Lambda"])

	// Wire Step Functions → SQS/SNS/DynamoDB/ECS/Glue/EventBridge/S3 service integrations.
	wireStepFunctionsServiceIntegrations(
		byName["StepFunctions"],
		byName["SQS"],
		byName["SNS"],
		byName["DynamoDB"],
		byName["ECS"],
		byName["Glue"],
		byName["EventBridge"],
		byName["S3"],
	)
}

// wireParameterAndKeyIntegrations wires SSM's KMS and EventBridge
// dependencies, and Kinesis's KMS dependency.
func wireParameterAndKeyIntegrations(byName map[string]service.Registerable) {
	// Wire SSM → KMS for SecureString encryption with customer-managed keys.
	wireSSMKMS(byName["SSM"], byName["KMS"])

	// Wire SSM → EventBridge so parameter-policy notifications are actually emitted.
	wireSSMParameterPolicyNotifications(byName["SSM"], byName["EventBridge"])

	// Wire Kinesis → KMS so StartStreamEncryption validates KeyId against real key state.
	wireKinesisKMS(byName["Kinesis"], byName["KMS"])
}

// wireAPIGatewayIntegrations wires API Gateway's Lambda proxy, Cognito
// authorizer, Cognito Lambda trigger, and WebSocket Management API
// integrations.
func wireAPIGatewayIntegrations(byName map[string]service.Registerable) {
	// Wire API Gateway → Lambda proxy integration.
	wireAPIGatewayLambda(byName["APIGateway"], byName["APIGatewayV2"], byName["Lambda"])

	// Wire API Gateway → Cognito for JWT signature verification.
	wireAPIGatewayCognito(byName["APIGateway"], byName["APIGatewayV2"], byName["CognitoIDP"])

	// Wire Cognito User Pool Lambda triggers (PreSignUp, PostConfirmation,
	// PreTokenGeneration, CustomMessage) to the Lambda backend.
	wireCognitoLambdaTriggers(byName["CognitoIDP"], byName["Lambda"])

	// Wire API Gateway V2 -> API Gateway Management API for WebSocket connections.
	wireAPIGatewayManagementAPI(byName["APIGatewayV2"], byName["APIGatewayManagementAPI"])
}

// wireEventSourcePollers wires the Kinesis, SQS, and DynamoDB Streams event
// source mapping pollers that invoke Lambda.
func wireEventSourcePollers(byName map[string]service.Registerable) {
	// Wire Kinesis → Lambda event source mapping poller.
	wireKinesisLambda(byName["Kinesis"], byName["Lambda"])

	// Wire SQS → Lambda event source mapping poller.
	wireSQSLambda(byName["SQS"], byName["Lambda"])

	// Wire DynamoDB Streams → Lambda event source mapping poller.
	wireDynamoDBStreamLambda(byName["DynamoDB"], byName["Lambda"])
}

// wireComputeAndObservabilityIntegrations wires Lambda async destinations,
// CloudWatch alarm/infra actions, Auto Scaling/ECS ELBv2 target
// registration, and CloudWatch Logs delivery from Lambda.
func wireComputeAndObservabilityIntegrations(appCtx *service.AppContext, byName map[string]service.Registerable) {
	// Wire Lambda async DeadLetterConfig / DestinationConfig delivery to SQS/SNS/Lambda.
	wireLambdaAsyncDestinations(byName["Lambda"], byName["SQS"], byName["SNS"])

	// Wire CloudWatch alarm actions → SNS, Lambda, EC2, and Auto Scaling backends.
	wireCloudWatchAlarmActions(byName["CloudWatch"], byName["SNS"], byName["Lambda"])
	wireCloudWatchInfraActions(
		byName["CloudWatch"], byName["EC2"], byName["Autoscaling"],
	)

	// Wire Auto Scaling → EC2 so scale-out launches real (mock) EC2 instances
	// and scale-in terminates them there too, instead of Auto Scaling
	// fabricating instance IDs with no EC2-side record.
	wireAutoScalingEC2(byName["Autoscaling"], byName["EC2"])

	// Wire Auto Scaling → ELBv2 so TargetGroupARNs membership changes
	// register/deregister real ELBv2 targets, instead of TargetGroupARNs
	// being stored and echoed with no effect on DescribeTargetHealth.
	wireAutoScalingELBv2(byName["Autoscaling"], byName["ELBv2"])

	// Wire ECS → ELBv2 so tasks belonging to a service with LoadBalancers
	// configured register/deregister as real ELBv2 targets as they
	// reach/leave RUNNING, instead of Service.LoadBalancers being stored and
	// echoed with no effect on DescribeTargetHealth.
	wireECSELBv2(byName["ECS"], byName["ELBv2"])

	// Wire CloudWatch Logs → Lambda log delivery.
	wireLambdaCWLogs(byName["Lambda"], byName["CloudWatchLogs"])

	// Wire Timestream Query → Timestream Write's shared tag store, so
	// CreateScheduledQuery's Tags reach TagResource/ListTagsForResource --
	// timestreamquery's own RouteMatcher defers those ops to TimestreamWrite
	// (handler.go's writeServiceTagOps), matching real Timestream's single
	// tag store shared across both API surfaces.
	wireTimestreamQueryTags(byName["TimestreamQuery"], byName["TimestreamWrite"])

	// Wire CloudWatch Logs subscription filter delivery to Lambda, Kinesis, and Firehose.
	wireCWLogsSubscriptionFilters(
		appCtx.JanitorCtx,
		byName["CloudWatchLogs"],
		byName["Lambda"],
		byName["Kinesis"],
		byName["Firehose"],
	)

	// Wire Direct Connect → EC2 so DirectConnectGatewayAssociation's
	// GatewayId/VirtualGatewayId are validated against real (mock) EC2
	// VpnGateway/TransitGateway records, and DescribeVirtualGateways proxies
	// EC2's own VpnGateway list instead of maintaining a duplicate store.
	wireDirectConnectEC2(byName["DirectConnect"], byName["EC2"])

	// Wire Network Manager → EC2/DirectConnect so cross-service ARNs
	// (CustomerGatewayArn/TransitGatewayArn/TransitGatewayConnectPeerArn/
	// VpcArn/SubnetArns/VpnConnectionArn/TransitGatewayRouteTableArn/
	// DirectConnectGatewayArn) are validated against real backend state
	// instead of accepted as opaque strings, and StartRouteAnalysis can walk
	// real EC2 Transit Gateway route-table state.
	wireNetworkManagerEC2(byName["NetworkManager"], byName["EC2"])
	wireNetworkManagerDirectConnect(byName["NetworkManager"], byName["DirectConnect"])

	// Wire ELB (Classic) → EC2/ACM/IAM so ApplySecurityGroupsToLoadBalancer/
	// AttachLoadBalancerToSubnets validate SecurityGroups/Subnets against
	// real EC2 state, and HTTPS/SSL listeners validate SSLCertificateId
	// against real ACM/IAM certificates, instead of accepting any string.
	wireELBCrossService(byName["ELB"], byName["EC2"], byName["ACM"], byName["IAM"])
}

// directConnectEC2ResolverAdapter adapts the EC2 backend to the
// directconnect.EC2GatewayResolver interface.
type directConnectEC2ResolverAdapter struct {
	backend *ec2backend.InMemoryBackend
}

func (a *directConnectEC2ResolverAdapter) ResolveVpnGateway(id string) bool {
	return len(a.backend.DescribeVpnGateways([]string{id})) > 0
}

func (a *directConnectEC2ResolverAdapter) ResolveTransitGateway(id string) bool {
	return len(a.backend.DescribeTransitGateways([]string{id})) > 0
}

func (a *directConnectEC2ResolverAdapter) VirtualGateways() []string {
	vgws := a.backend.DescribeVpnGateways(nil)
	ids := make([]string, 0, len(vgws))

	for _, v := range vgws {
		ids = append(ids, v.VpnGatewayID)
	}

	return ids
}

// wireDirectConnectEC2 wires the Direct Connect backend to the EC2 backend
// -- see directConnectEC2ResolverAdapter.
func wireDirectConnectEC2(directconnectReg, ec2Reg service.Registerable) {
	directconnectH, ok := directconnectReg.(*directconnectbackend.Handler)
	if !ok {
		return
	}

	ec2H, ok := ec2Reg.(*ec2backend.Handler)
	if !ok {
		return
	}

	ec2Bk, ok := ec2H.Backend.(*ec2backend.InMemoryBackend)
	if !ok {
		return
	}

	directconnectH.Backend.SetEC2GatewayResolver(&directConnectEC2ResolverAdapter{backend: ec2Bk})
}

// arnResourceID extracts the trailing resource-id segment of an ARN's
// resource part (e.g. "arn:aws:ec2:us-east-1:123456789012:vpc/vpc-0123"
// -> "vpc-0123"), the shape every EC2/DirectConnect resource kind
// networkManagerEC2ResolverAdapter/networkManagerDirectConnectResolverAdapter
// look up by. A bare (non-ARN) id string passes through unchanged.
func arnResourceID(arnStr string) string {
	if i := strings.LastIndex(arnStr, "/"); i >= 0 {
		return arnStr[i+1:]
	}

	return arnStr
}

// networkManagerEC2ResolverAdapter adapts the EC2 backend to the
// networkmanager.EC2Resolver interface.
type networkManagerEC2ResolverAdapter struct {
	backend *ec2backend.InMemoryBackend
}

func (a *networkManagerEC2ResolverAdapter) ResolveVpc(vpcArn string) bool {
	return len(a.backend.DescribeVpcs([]string{arnResourceID(vpcArn)})) > 0
}

func (a *networkManagerEC2ResolverAdapter) ResolveSubnet(subnetArn string) bool {
	return len(a.backend.DescribeSubnets([]string{arnResourceID(subnetArn)})) > 0
}

func (a *networkManagerEC2ResolverAdapter) ResolveCustomerGateway(customerGatewayArn string) bool {
	return len(a.backend.DescribeCustomerGateways([]string{arnResourceID(customerGatewayArn)})) > 0
}

func (a *networkManagerEC2ResolverAdapter) ResolveTransitGateway(transitGatewayArn string) bool {
	return len(a.backend.DescribeTransitGateways([]string{arnResourceID(transitGatewayArn)})) > 0
}

func (a *networkManagerEC2ResolverAdapter) ResolveVpnConnection(vpnConnectionArn string) bool {
	return len(a.backend.DescribeVpnConnections([]string{arnResourceID(vpnConnectionArn)})) > 0
}

func (a *networkManagerEC2ResolverAdapter) ResolveTransitGatewayConnectPeer(transitGatewayConnectPeerArn string) bool {
	return len(a.backend.DescribeTransitGatewayConnectPeers([]string{arnResourceID(transitGatewayConnectPeerArn)})) > 0
}

func (a *networkManagerEC2ResolverAdapter) ResolveTransitGatewayRouteTable(transitGatewayRouteTableArn string) bool {
	return len(a.backend.DescribeTransitGatewayRouteTables([]string{arnResourceID(transitGatewayRouteTableArn)})) > 0
}

// TransitGatewayRouteTableForAttachment resolves a TGW VPC attachment to
// the route table it is associated with, by scanning its owning transit
// gateway's route tables for an association naming this attachment --
// services/ec2 has no direct "route table for attachment" index, only the
// reverse (GetTransitGatewayRouteTableAssociations(routeTableID)).
func (a *networkManagerEC2ResolverAdapter) TransitGatewayRouteTableForAttachment(
	transitGatewayAttachmentArn string,
) (string, bool) {
	attachmentID := arnResourceID(transitGatewayAttachmentArn)

	atts := a.backend.DescribeTransitGatewayVpcAttachments([]string{attachmentID})
	if len(atts) == 0 || atts[0].State != "available" {
		return "", false
	}

	for _, rt := range a.backend.DescribeTransitGatewayRouteTables(nil) {
		if rt.TransitGatewayID != atts[0].TransitGatewayID {
			continue
		}

		assocs, err := a.backend.GetTransitGatewayRouteTableAssociations(rt.RouteTableID)
		if err != nil {
			continue
		}

		for _, assoc := range assocs {
			if assoc.TransitGatewayAttachmentID == attachmentID {
				return rt.RouteTableID, true
			}
		}
	}

	return "", false
}

func (a *networkManagerEC2ResolverAdapter) TransitGatewayRoutes(
	routeTableID string,
) []networkmanagerbackend.EC2TransitGatewayRoute {
	routes, err := a.backend.SearchTransitGatewayRoutes(routeTableID, nil)
	if err != nil {
		return nil
	}

	out := make([]networkmanagerbackend.EC2TransitGatewayRoute, 0, len(routes))

	for _, r := range routes {
		out = append(out, networkmanagerbackend.EC2TransitGatewayRoute{
			DestinationCIDRBlock: r.DestinationCidrBlock,
			State:                r.State,
			AttachmentID:         r.TransitGatewayAttachmentID,
		})
	}

	return out
}

// wireNetworkManagerEC2 wires the Network Manager backend to the EC2
// backend -- see networkManagerEC2ResolverAdapter. Validates
// CustomerGatewayArn/TransitGatewayArn/TransitGatewayConnectPeerArn/VpcArn/
// SubnetArns/VpnConnectionArn/TransitGatewayRouteTableArn against real EC2
// state instead of accepting any string, and lets StartRouteAnalysis walk
// real EC2 Transit Gateway route-table state.
func wireNetworkManagerEC2(networkmanagerReg, ec2Reg service.Registerable) {
	networkmanagerH, ok := networkmanagerReg.(*networkmanagerbackend.Handler)
	if !ok {
		return
	}

	ec2H, ok := ec2Reg.(*ec2backend.Handler)
	if !ok {
		return
	}

	ec2Bk, ok := ec2H.Backend.(*ec2backend.InMemoryBackend)
	if !ok {
		return
	}

	networkmanagerH.Backend.SetEC2Resolver(&networkManagerEC2ResolverAdapter{backend: ec2Bk})
}

// networkManagerDirectConnectResolverAdapter adapts the DirectConnect
// backend to the networkmanager.DirectConnectResolver interface.
type networkManagerDirectConnectResolverAdapter struct {
	backend *directconnectbackend.InMemoryBackend
}

func (a *networkManagerDirectConnectResolverAdapter) ResolveDirectConnectGateway(
	directConnectGatewayArn string,
) bool {
	return len(a.backend.DescribeDirectConnectGateways(arnResourceID(directConnectGatewayArn))) > 0
}

// wireNetworkManagerDirectConnect wires the Network Manager backend to the
// DirectConnect backend -- see networkManagerDirectConnectResolverAdapter.
// Validates DirectConnectGatewayArn against real DirectConnect state
// instead of accepting any string.
func wireNetworkManagerDirectConnect(networkmanagerReg, directconnectReg service.Registerable) {
	networkmanagerH, ok := networkmanagerReg.(*networkmanagerbackend.Handler)
	if !ok {
		return
	}

	directconnectH, ok := directconnectReg.(*directconnectbackend.Handler)
	if !ok {
		return
	}

	networkmanagerH.Backend.SetDirectConnectResolver(
		&networkManagerDirectConnectResolverAdapter{backend: directconnectH.Backend},
	)
}

// elbEC2ResolverAdapter adapts the EC2 backend to the elb.EC2Resolver interface.
type elbEC2ResolverAdapter struct {
	backend ec2backend.Backend
}

func (a *elbEC2ResolverAdapter) SecurityGroupExists(id string) bool {
	return len(a.backend.DescribeSecurityGroups([]string{id})) > 0
}

func (a *elbEC2ResolverAdapter) SubnetExists(id string) bool {
	return len(a.backend.DescribeSubnets([]string{id})) > 0
}

// elbCertificateResolverAdapter adapts the ACM and IAM backends to the
// elb.CertificateResolver interface. AWS accepts either an ACM or an IAM
// server-certificate ARN for SSLCertificateId (see elb.CertificateResolver's
// doc comment), so both are consulted; either backend may be nil if that
// service isn't registered.
type elbCertificateResolverAdapter struct {
	acmBackend *acmbackend.InMemoryBackend
	iamBackend *iambackend.InMemoryBackend
}

func (a *elbCertificateResolverAdapter) ResolveCertificate(ctx context.Context, certARN string) bool {
	if a.acmBackend != nil {
		if _, err := a.acmBackend.DescribeCertificate(ctx, certARN); err == nil {
			return true
		}
	}

	if a.iamBackend != nil {
		certs, err := a.iamBackend.ListServerCertificates("")
		if err == nil {
			for _, c := range certs {
				if c.Arn == certARN {
					return true
				}
			}
		}
	}

	return false
}

// wireELBCrossService wires the Classic ELB backend to EC2 (SecurityGroups/
// Subnets existence) and ACM/IAM (SSLCertificateId existence) -- see
// elb.EC2Resolver/elb.CertificateResolver's doc comments.
func wireELBCrossService(elbReg, ec2Reg, acmReg, iamReg service.Registerable) {
	elbH, ok := elbReg.(*elbbackend.Handler)
	if !ok {
		return
	}

	elbBk, bkOk := elbH.Backend.(*elbbackend.InMemoryBackend)
	if !bkOk {
		return
	}

	if ec2H, ec2Ok := ec2Reg.(*ec2backend.Handler); ec2Ok {
		elbBk.SetEC2Resolver(&elbEC2ResolverAdapter{backend: ec2H.Backend})
	}

	var acmBk *acmbackend.InMemoryBackend
	if acmH, acmOk := acmReg.(*acmbackend.Handler); acmOk {
		acmBk = acmH.Backend
	}

	var iamBk *iambackend.InMemoryBackend
	if iamH, iamOk := iamReg.(*iambackend.Handler); iamOk {
		iamBk, _ = iamH.Backend.(*iambackend.InMemoryBackend)
	}

	if acmBk != nil || iamBk != nil {
		elbBk.SetCertificateResolver(&elbCertificateResolverAdapter{acmBackend: acmBk, iamBackend: iamBk})
	}
}

// wireCWLogsMetricEmitters wires CloudWatch Logs metric filters to emit
// CloudWatch metric data points. The repeated identical calls are preserved
// verbatim from before this decomposition; collapsing them is a behavior
// change outside the scope of this refactor.
func wireCWLogsMetricEmitters(byName map[string]service.Registerable) {
	// Wire CloudWatch Logs metric filters to emit CloudWatch metric data points.
	wireCWLogsMetricEmitter(byName["CloudWatchLogs"], byName["CloudWatch"])

	// Wire CloudWatch Logs metric filters to emit CloudWatch metric data points.
	wireCWLogsMetricEmitter(byName["CloudWatchLogs"], byName["CloudWatch"])

	// Wire CloudWatch Logs metric filters to emit CloudWatch metric data points.
	wireCWLogsMetricEmitter(byName["CloudWatchLogs"], byName["CloudWatch"])

	// Wire CloudWatch Logs metric filters to emit CloudWatch metric data points.
	wireCWLogsMetricEmitter(byName["CloudWatchLogs"], byName["CloudWatch"])

	// Wire CloudWatch Logs metric filters to emit CloudWatch metric data points.
	wireCWLogsMetricEmitter(byName["CloudWatchLogs"], byName["CloudWatch"])

	// Wire CloudWatch Logs metric filters to emit CloudWatch metric data points.
	wireCWLogsMetricEmitter(byName["CloudWatchLogs"], byName["CloudWatch"])

	// Wire CloudWatch Logs metric filters to emit CloudWatch metric data points.
	wireCWLogsMetricEmitter(byName["CloudWatchLogs"], byName["CloudWatch"])

	// Wire CloudWatch Logs metric filters to emit CloudWatch metric data points.
	wireCWLogsMetricEmitter(byName["CloudWatchLogs"], byName["CloudWatch"])

	// Wire CloudWatch Logs metric filters to emit CloudWatch metric data points.
	wireCWLogsMetricEmitter(byName["CloudWatchLogs"], byName["CloudWatch"])

	// Wire CloudWatch Logs metric filters to emit CloudWatch metric data points.
	wireCWLogsMetricEmitter(byName["CloudWatchLogs"], byName["CloudWatch"])

	// Wire CloudWatch Logs metric filters to emit CloudWatch metric data points.
	wireCWLogsMetricEmitter(byName["CloudWatchLogs"], byName["CloudWatch"])

	// Wire CloudWatch Logs metric filters to emit CloudWatch metric data points.
	wireCWLogsMetricEmitter(byName["CloudWatchLogs"], byName["CloudWatch"])

	// Wire CloudWatch Logs metric filters to emit CloudWatch metric data points.
	wireCWLogsMetricEmitter(byName["CloudWatchLogs"], byName["CloudWatch"])

	// Wire CloudWatch Logs metric filters to emit CloudWatch metric data points.
	wireCWLogsMetricEmitter(byName["CloudWatchLogs"], byName["CloudWatch"])

	// Wire CloudWatch Logs metric filters to emit CloudWatch metric data points.
	wireCWLogsMetricEmitter(byName["CloudWatchLogs"], byName["CloudWatch"])

	// Wire CloudWatch Logs metric filters to emit CloudWatch metric data points.
	wireCWLogsMetricEmitter(byName["CloudWatchLogs"], byName["CloudWatch"])

	// Wire CloudWatch Logs metric filters to emit CloudWatch metric data points.
	wireCWLogsMetricEmitter(byName["CloudWatchLogs"], byName["CloudWatch"])
}

// wireStorageAndSecretsIntegrations wires Firehose delivery, DynamoDB→S3
// import/export, MGN→S3 import, SageMaker→S3 pipeline definitions,
// SecretsManager's Lambda rotation invoker and KMS encryption, and IoT rule
// action dispatch.
func wireStorageAndSecretsIntegrations(byName map[string]service.Registerable) {
	// Wire Firehose → S3 and Lambda for actual record delivery and transformation.
	wireFirehoseDelivery(byName["Firehose"], byName["S3"], byName["Lambda"])

	// Wire DynamoDB → S3 so ImportTable reads source objects and
	// ExportTableToPointInTime writes real export data.
	wireDynamoDBS3(byName["DynamoDB"], byName["S3"])

	// Wire MGN → S3 so StartImport reads its caller-supplied S3 object and
	// actually creates SourceServers.
	wireMGNS3(byName["MGN"], byName["S3"])

	// Wire SageMaker → S3 so CreatePipeline/UpdatePipeline can fetch a
	// PipelineDefinitionS3Location's object as the real pipeline definition.
	wireSageMakerS3(byName["SageMaker"], byName["S3"])

	// Wire Glacier → S3 so a completed Select job writes its real
	// OutputLocation output (job.txt/results/result_manifest.txt) instead of
	// only serving results via GetJobOutput.
	wireGlacierS3(byName["Glacier"], byName["S3"])

	// Wire Lambda invoker → SecretsManager rotation.
	wireSecretsManagerLambda(byName["SecretsManager"], byName["Lambda"])

	// Wire SecretsManager → KMS so secret values are encrypted/decrypted via
	// the real KMS backend instead of stored opaquely.
	wireSecretsManagerKMS(byName["SecretsManager"], byName["KMS"])

	// Wire IoT rules → SQS/Lambda action dispatch, and broker → IoT Data Plane.
	wireIoTRules(byName["IoT"], byName["IoTDataPlane"], byName["SQS"], byName["Lambda"])

	// Wire IoT Analytics' RunPipelineActivity lambda/deviceRegistryEnrich/
	// deviceShadowEnrich activities to the real Lambda and IoT backends.
	wireIoTAnalyticsCrossService(byName["IoTAnalytics"], byName["Lambda"], byName["IoT"])

	// Wire Kinesis Analytics' DiscoverInputSchema to the real Kinesis and S3 backends so it
	// samples real records instead of returning UnableToDetectSchemaException for every
	// well-formed request. Firehose delivery streams stay unreachable as a source (see
	// services/kinesisanalytics/PARITY.md's known gaps).
	wireKinesisAnalyticsCrossService(byName["KinesisAnalytics"], byName["Kinesis"], byName["S3"])

	// Wire AppConfig → AppConfigData so a completed deployment's
	// configuration becomes observable through GetLatestConfiguration polling.
	wireAppConfigDeployments(byName["AppConfig"], byName["AppConfigData"])
}

// wireAppConfigDeployments wires the AppConfigData backend as AppConfig's
// DeployedConfigurationPublisher: once a deployment reaches COMPLETE (or a
// StopDeployment AllowRevert restores a prior version), its configuration is
// pushed into AppConfigData so a real StartConfigurationSession +
// GetLatestConfiguration poll observes it. appconfigdatabackend.InMemoryBackend
// satisfies appconfigbackend.DeployedConfigurationPublisher directly (same
// no-adapter pairing as cloudwatch's FirehosePutter/firehose.InMemoryBackend).
func wireAppConfigDeployments(appconfigReg, appconfigdataReg service.Registerable) {
	acH, ok := appconfigReg.(*appconfigbackend.Handler)
	if !ok {
		return
	}

	acBk, ok := acH.Backend.(*appconfigbackend.InMemoryBackend)
	if !ok {
		return
	}

	acdH, ok := appconfigdataReg.(*appconfigdatabackend.Handler)
	if !ok || acdH.Backend == nil {
		return
	}

	acBk.SetDeployedConfigurationPublisher(acdH.Backend)
}

// wireAppSyncAndStreamsIntegrations wires AppSync's Lambda and DynamoDB
// resolvers, DynamoDB Streams to the DynamoDB backend, and CloudFront
// KeyValueStore to the CloudFront backend.
func wireAppSyncAndStreamsIntegrations(byName map[string]service.Registerable) {
	// Wire AppSync → Lambda for LAMBDA resolver execution.
	wireAppSyncLambda(byName["AppSync"], byName["Lambda"])

	// Wire AppSync → DynamoDB for AMAZON_DYNAMODB resolver execution.
	wireAppSyncDynamoDB(byName["AppSync"], byName["DynamoDB"])

	// Wire DynamoDB Streams → DynamoDB backend so streams share the same in-memory data.
	wireDynamoDBStreams(byName["DynamoDB"], byName["DynamoDBStreams"])

	// Wire CloudFront KeyValueStore → CloudFront backend so the data-plane ops
	// (GetKey/PutKey/DeleteKey/ListKeys/UpdateKeys/DescribeKeyValueStore) act on
	// the same KVS stores the CloudFront control-plane ops manage.
	wireCloudFrontKeyValueStore(byName["CloudFront"], byName["CloudFront KeyValueStore"])
}

// wireSchedulerAndPipesIntegrations wires the Scheduler and Pipes runners
// to the backends they can target.
func wireSchedulerAndPipesIntegrations(byName map[string]service.Registerable) {
	// Wire Scheduler runner → Lambda, SQS, SNS, and StepFunctions backends.
	wireSchedulerRunner(
		byName["Scheduler"],
		byName["Lambda"],
		byName["SQS"],
		byName["SNS"],
		byName["StepFunctions"],
		byName["EventBridge"],
		byName["Kinesis"],
		byName["SageMaker"],
		byName["ECS"],
	)

	// Wire Pipes runner → SQS/Kinesis/DynamoDB Streams (sources), and
	// Lambda/StepFunctions/SNS/SQS/Kinesis/EventBridge/CloudWatchLogs/Firehose
	// (targets + DLQ).
	wirePipesRunner(
		byName["Pipes"],
		byName["SQS"],
		byName["Lambda"],
		byName["StepFunctions"],
		byName["SNS"],
		byName["Kinesis"],
		byName["EventBridge"],
		byName["CloudWatchLogs"],
		byName["Firehose"],
		byName["DynamoDB"],
	)
}

// wireGovernanceIntegrations wires Resource Groups Tagging API aggregation
// across service backends, IAM→STS validation, and FIS action provider
// registration.
func wireGovernanceIntegrations(byName map[string]service.Registerable, services []service.Registerable) {
	// Wire Resource Groups Tagging API → service backends so GetResources, TagResources, etc.
	// aggregate and mutate tags across all services.
	wireResourceGroupsTagging(byName["ResourceGroupsTaggingAPI"], byName)

	// Wire IAM → STS for ExternalId validation and MaxSessionDuration enforcement.
	wireIAMToSTS(byName["IAM"], byName["STS"])

	// Collect all services implementing FISActionProvider and register them with the FIS backend.
	wireFISActionProviders(byName["FIS"], services)
}

// registerCloudFormationAndDashboard registers CloudFormation and the
// dashboard, both of which need the full set of already-initialized service
// handlers: CloudFormation to drive stack resources through their real
// backends, and the dashboard to expose them. CloudFormation is registered
// first so its handler is stored before the dashboard, which is
// initialized last, reads it. byName is the pre-CloudFormation lookup built
// before wireCrossServiceDependencies -- used here to wire the one
// direction that depends on CloudFormation existing (Lightsail ->
// CloudFormation), which wireCrossServiceDependencies itself runs too early
// for since CloudFormation isn't registered yet at that point.
func registerCloudFormationAndDashboard(
	appCtx *service.AppContext,
	services []service.Registerable,
	byName map[string]service.Registerable,
) ([]service.Registerable, error) {
	// Init CloudFormation after core handlers are stored so it can access their backends.
	cfnSvc, err := (&cfnbackend.Provider{}).Init(appCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to init CloudFormation: %w", err)
	}

	services = append(services, cfnSvc)

	// Wire Lightsail -> CloudFormation so CreateCloudFormationStack's real
	// cross-service handoff actually creates a stack instead of always
	// falling back to a record-only stub. Must happen here, not in
	// wireStorageAndSecretsIntegrations, because CloudFormation does not
	// exist yet when that runs.
	wireLightsailCloudFormation(byName["Lightsail"], cfnSvc)

	// Wire CloudFormation -> Organizations so SERVICE_MANAGED StackSets can
	// expand DeploymentTargets.OrganizationalUnitIds against the real OU
	// hierarchy instead of rejecting OU-based targets outright.
	wireCloudFormationOrganizations(cfnSvc, byName["Organizations"])

	if cli, ok := appCtx.Config.(*CLI); ok {
		cli.cloudFormationHandler = cfnSvc
	}

	// Init dashboard last so it can access all service handlers.
	dashSvc, err := (&dashboard.Provider{}).Init(appCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to init Dashboard: %w", err)
	}
	services = append(services, dashSvc)

	return services, nil
}

// getServiceProviders returns the list of all available service providers.
func getServiceProviders() []service.Provider {
	return append(getCoreServiceProviders(), getRemainingServiceProviders()...)
}

// getCoreServiceProviders returns the foundational service providers.
// Extracted from getServiceProviders to satisfy the funlen limit and to
// give the inline registration list headroom as new services are added.
func getCoreServiceProviders() []service.Provider {
	return []service.Provider{
		&ddbbackend.Provider{},
		&s3backend.Provider{},
		&ssmbackend.Provider{},
		&iambackend.Provider{},
		&stsbackend.Provider{},
		&snsbackend.Provider{},
		&sqsbackend.Provider{},
		&kmsbackend.Provider{},
		&secretsmanagerbackend.Provider{},
		&lambdabackend.Provider{},
		&ebbackend.Provider{},
	}
}

// getRemainingServiceProviders returns the remaining service providers. New
// services should be registered in getMostRecentServiceProviders (the tail of
// the chain), not here, to keep this function under the funlen limit.
func getRemainingServiceProviders() []service.Provider {
	return append([]service.Provider{
		&apigwbackend.Provider{},
		&cwlogsbackend.Provider{},
		&sfnbackend.Provider{},
		&cwbackend.Provider{},
		&kinesisbackend.Provider{},
		&elasticachebackend.Provider{},
		&route53backend.Provider{},
		&sesbackend.Provider{},
		&sesv2backend.Provider{},
		&ec2backend.Provider{},
		&opensearchbackend.Provider{},
		&acmbackend.Provider{},
		&acmpcabackend.Provider{},
		&redshiftbackend.Provider{},
		&redshiftbackend.ServerlessProvider{},
		&awsconfigbackend.Provider{},
		&s3controlbackend.Provider{},
		&resourcegroupsbackend.Provider{},
		&resourcegroupstaggingapibackend.Provider{},
		&swfbackend.Provider{},
		&firehosebackend.Provider{},
		&networkmonitorbackend.Provider{},
		&schedulerbackend.Provider{},
		&route53resolverbackend.Provider{},
		&rdsbackend.Provider{},
		&transcribebackend.Provider{},
		&pollybackend.Provider{},
		&supportbackend.Provider{},
		&ecrbackend.Provider{},
		&ecsbackend.Provider{},
		&fisbackend.Provider{},
		&identitystorebackend.Provider{},
		&organizationsbackend.Provider{},
		&cognitoidpbackend.Provider{},
		&cognitoidentitybackend.Provider{},
		&iotbackend.Provider{},
		&iotdataplanebackend.Provider{},
		&appsyncbackend.Provider{},
		&apigwmgmtbackend.Provider{},
		&appconfigdatabackend.Provider{},
		&amplifybackend.Provider{},
		&autoscalingbackend.Provider{},
		&apigwv2backend.Provider{},
		&athenabackend.Provider{},
		&appconfigbackend.Provider{},
		&backupbackend.Provider{},
		&cloudtrailbackend.Provider{},
		&applicationautoscalingbackend.Provider{},
		&batchbackend.Provider{},
		&bedrockbackend.Provider{},
		&bedrockbackend.AgentsProvider{},
		&bedrockruntimebackend.Provider{},
		&cebackend.Provider{},
		&cloudcontrolbackend.Provider{},
		&cloudfrontbackend.Provider{},
		&cfkvsbackend.Provider{},
		&codeartifactbackend.Provider{},
		&codebuildbackend.Provider{},
		&codecommitbackend.Provider{},
		&codepipelinebackend.Provider{},
		&codeconnectionsbackend.Provider{},
		&codedeploybackend.Provider{},
		&dmsbackend.Provider{},
		&codestarconnectionsbackend.Provider{},
		&dynamodbstreamsbackend.Provider{},
		&elasticbeanstalkbackend.Provider{},
		&elasticsearchbackend.Provider{},
		&efsbackend.Provider{},
		&eksbackend.Provider{},
		&elbbackend.Provider{},
		&elbv2backend.Provider{},
		&emrserverlessbackend.Provider{},
		&emrbackend.Provider{},
		&gluebackend.Provider{},
		&guarddutybackend.Provider{},
		&inspector2backend.Provider{},
		&docdbbackend.Provider{},
		&glacierbackend.Provider{},
		&iotanalyticsbackend.Provider{},
		&iotwirelessbackend.Provider{},
		&kinesisanalyticsbackend.Provider{},
		&kafkabackend.Provider{},
		&kinesisanalyticsv2backend.Provider{},
		&lakeformationbackend.Provider{},
		&managedblockchainbackend.Provider{},
		&mediaconvertbackend.Provider{},
		&mqbackend.Provider{},
		&mediastorebackend.Provider{},
	}, getLatestServiceProviders()...)
}

// getLatestServiceProviders returns providers for additional services.
// Extracted from getServiceProviders to satisfy the funlen limit.
func getLatestServiceProviders() []service.Provider {
	return append([]service.Provider{
		&mediastoredatabackend.Provider{},
		&memorydbbackend.Provider{},
	}, getNewestServiceProviders()...)
}

// getNewestServiceProviders returns the most recently added service providers.
// Extracted from getServiceProviders to satisfy the funlen limit.
func getNewestServiceProviders() []service.Provider {
	return append([]service.Provider{
		&mwaabackend.Provider{},
		&neptunebackend.Provider{},
	}, getMostRecentServiceProviders()...)
}

func getMostRecentServiceProviders() []service.Provider {
	return []service.Provider{
		&azureblobbackend.Provider{},
		&azurequeuebackend.Provider{},
		&azuretablebackend.Provider{},
		&azureservicebusbackend.Provider{},
		&cosmosdbbackend.Provider{},
		&pinpointbackend.Provider{},
		&pipesbackend.Provider{},
		&accessanalyzerbackend.Provider{},
		&accountbackend.Provider{},
		&rambackend.Provider{},
		&rolesanywherebackend.Provider{},
		&rdsdatabackend.Provider{},
		&redshiftdatabackend.Provider{},
		&sagemakerbackend.Provider{},
		&sagemakerruntimebackend.Provider{},
		&servicediscoverybackend.Provider{},
		&serverlessrepobackend.Provider{},
		&shieldbackend.Provider{},
		&ssoadminbackend.Provider{},
		&textractbackend.Provider{},
		&comprehendbackend.Provider{},
		&timestreamwritebackend.Provider{},
		&timestreamquerybackend.Provider{},
		&transferbackend.Provider{},
		&verifiedpermissionsbackend.Provider{},
		&wafbackend.Provider{},
		&wafv2backend.Provider{},
		&workmailbackend.Provider{},
		&workspacesbackend.Provider{},
		&xraybackend.Provider{},
		&s3tablesbackend.Provider{},
		&databrewbackend.Provider{},
		&cleanroomsbackend.Provider{},
		&directoryservicebackend.Provider{},
		&forecastbackend.Provider{},
		&mediatailorbackend.Provider{},
		&macie2backend.Provider{},
		&apprunnerbackend.Provider{},
		&appmeshbackend.Provider{},
		&appstreambackend.Provider{},
		&detectivebackend.Provider{},
		&datasyncbackend.Provider{},
		&opsworksbackend.Provider{},
		&dlmbackend.Provider{},
		&fsxbackend.Provider{},
		&daxbackend.Provider{},
		&medialivebackend.Provider{},
		&mediapackagebackend.Provider{},
		&personalizebackend.Provider{},
		&quicksightbackend.Provider{},
		&rekognitionbackend.Provider{},
		&translatebackend.Provider{},
		&securityhubbackend.Provider{},
		&vpclatticebackend.Provider{},
		&omicsbackend.Provider{},
		&bedrockagentbackend.Provider{},
		&grafanabackend.Provider{},
		&outpostsbackend.Provider{},
		&resiliencehubbackend.Provider{},
		&directconnectbackend.Provider{},
		&mgnbackend.Provider{},
		&networkmanagerbackend.Provider{},
		&lightsailbackend.Provider{},
	}
}

// startPurgeWorker runs the auto-purge ticker loop.
// It calls Purge on every service.Purgeable with a per-service timeout context
// so a slow or deadlocked backend cannot stall the entire purge cycle.
// startPurgeWorker periodically checks for and deletes resources older than the configured TTL.
// It dynamically reads the TTL from the global configuration, allowing runtime updates.
func startPurgeWorker(
	ctx context.Context,
	gcfg *config.GlobalConfig,
	svcs []service.Registerable,
) {
	// Tag this background routine so its records are attributable (worker=purge-worker).
	ctx = logger.WithWorker(ctx, "purge", "worker")
	log := logger.Load(ctx)

	const (
		purgeTimeout  = 30 * time.Second
		checkInterval = 10 * time.Second
	)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	var lastPurgeAt time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ttl := gcfg.GetAutoPurgeTTL()
			if ttl <= 0 {
				continue
			}

			// Avoid purging too frequently if the interval is short.
			// Only purge if at least 'ttl' has passed since the last start,
			// or if we've never purged.
			if !lastPurgeAt.IsZero() && time.Since(lastPurgeAt) < ttl {
				continue
			}

			lastPurgeAt = time.Now().UTC()
			cutoff := lastPurgeAt.Add(-ttl)
			log.InfoContext(ctx, "running automatic service purge", "ttl", ttl, "cutoff", cutoff)
			purgeAllServices(ctx, log, svcs, cutoff, purgeTimeout)
		}
	}
}

// purgeAllServices calls Purge on each Purgeable service with a timeout context.
func purgeAllServices(
	ctx context.Context,
	log *slog.Logger,
	svcs []service.Registerable,
	cutoff time.Time,
	timeout time.Duration,
) {
	const goroutineGracePeriod = 5 * time.Second

	for _, svc := range svcs {
		p, ok := svc.(service.Purgeable)
		if !ok {
			continue
		}
		purgeCtx, cancel := context.WithTimeout(ctx, timeout)
		done := make(chan struct{})
		go func() {
			defer close(done)
			p.Purge(purgeCtx, cutoff)
		}()
		select {
		case <-done:
			cancel()
		case <-purgeCtx.Done():
			cancel()
			if ctx.Err() == nil {
				log.WarnContext(ctx, "purge timed out", "service", svc.Name(), "timeout", timeout)
			}
			// Drain the goroutine with a grace period so it does not accumulate
			// across repeated purge cycles.
			graceTimer := time.NewTimer(goroutineGracePeriod)
			select {
			case <-done:
				graceTimer.Stop()
			case <-graceTimer.C:
				log.WarnContext(ctx, "purge goroutine did not exit after grace period",
					"service", svc.Name())
			}
		}
	}
}

// startBackgroundWorkers starts all background workers from services.
func startBackgroundWorkers(ctx context.Context, services []service.Registerable) {
	log := logger.Load(ctx)

	for _, svc := range services {
		if worker, ok := svc.(service.BackgroundWorker); ok {
			// Tag the worker's context so every record it emits is attributable
			// (service=<name> worker=<name>-worker). Workers that run finer-grained
			// jobs may further refine this with logger.WithWorker at their entry.
			wctx := logger.WithWorker(ctx, svc.Name(), "worker")
			if workerErr := worker.StartWorker(wctx); workerErr != nil {
				log.ErrorContext(wctx, "failed to start background worker", "error", workerErr)
			}
		}
	}
}

// shutdownServices calls Shutdown on every service that implements service.Shutdowner.
// All shutdowns run concurrently. shutdownServices blocks until all complete or ctx
// expires (whichever comes first), logging a warning if the deadline is exceeded.
func shutdownServices(ctx context.Context, services []service.Registerable) {
	log := logger.Load(ctx)

	var wg sync.WaitGroup

	for _, svc := range services {
		if s, ok := svc.(service.Shutdowner); ok {
			wg.Go(func() {
				log.InfoContext(ctx, "shutting down service", "service", svc.Name())
				s.Shutdown(ctx)
			})
		}
	}

	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		log.WarnContext(
			ctx,
			"service shutdown timed out; some background goroutines may still be running",
		)
	}
}

// wireSNSToSQS connects the SNS publish emitter to the SQS delivery handler so
// that messages published to SNS topics are delivered to subscribed SQS queues.
// snsReg and sqsReg must be the service.Registerable values returned by their
// respective providers (indices 5 and 6 in the services slice).
func wireSNSToSQS(snsReg, sqsReg service.Registerable) {
	snsH, ok1 := snsReg.(*snsbackend.Handler)
	sqsH, ok2 := sqsReg.(*sqsbackend.Handler)

	if !ok1 || !ok2 {
		return
	}

	snsBk, ok3 := snsH.Backend.(*snsbackend.InMemoryBackend)
	sqsBk, ok4 := sqsH.Backend.(*sqsbackend.InMemoryBackend)

	if !ok3 || !ok4 {
		return
	}

	emitter := snsevents.NewInMemoryEmitter[*snsevents.SNSPublishedEvent]()
	snsBk.SetPublishEmitter(emitter)
	sqsBk.SubscribeToSNS(emitter)
}

// wireSNSToLambdaFirehose connects the SNS backend to Lambda and Firehose so that
// Lambda- and Firehose-protocol subscriptions actually receive published messages,
// and wires an SQS sender so failed Lambda/Firehose deliveries with a RedrivePolicy
// land in the subscription's dead-letter queue. This is independent of wireSNSToSQS:
// that function wires the SNS→SQS *subscription* delivery path via a publish emitter,
// while the SQS sender wired here only serves DLQ redelivery on failed Lambda/Firehose
// invocations (SNS backend's sqsSender field), so there is no overlap or double-wiring.
func wireSNSToLambdaFirehose(snsReg, lambdaReg, firehoseReg, sqsReg service.Registerable) {
	snsH, ok := snsReg.(*snsbackend.Handler)
	if !ok {
		return
	}

	snsBk, ok := snsH.Backend.(*snsbackend.InMemoryBackend)
	if !ok {
		return
	}

	if lambdaH, lambdaOk := lambdaReg.(*lambdabackend.Handler); lambdaOk {
		if lambdaBk, bkOk := lambdaH.Backend.(*lambdabackend.InMemoryBackend); bkOk {
			snsBk.SetLambdaBackend(lambdaBk)
		}
	}

	if firehoseH, firehoseOk := firehoseReg.(*firehosebackend.Handler); firehoseOk {
		if firehoseBk, bkOk := firehoseH.Backend.(*firehosebackend.InMemoryBackend); bkOk {
			snsBk.SetFirehoseBackend(&snsFirehosePutterAdapter{backend: firehoseBk})
		}
	}

	if sqsH, sqsOk := sqsReg.(*sqsbackend.Handler); sqsOk {
		if sqsBk, bkOk := sqsH.Backend.(*sqsbackend.InMemoryBackend); bkOk {
			snsBk.SetSQSSender(&sqsSenderAdapter{backend: sqsBk})
		}
	}
}

// snsFirehosePutterAdapter adapts the Firehose backend to the sns.FirehosePutter
// interface, which omits the context parameter that the Firehose backend's
// PutRecordBatch requires.
type snsFirehosePutterAdapter struct {
	backend *firehosebackend.InMemoryBackend
}

func (a *snsFirehosePutterAdapter) PutRecordBatch(streamName string, records [][]byte) (int, error) {
	return a.backend.PutRecordBatch(context.Background(), streamName, records)
}

// wireSQSMetrics wires the CloudWatch metric emitter into the SQS backend so that
// SendMessage, ReceiveMessage, and DeleteMessage operations emit CloudWatch metrics.
func wireSQSMetrics(sqsReg, cwReg service.Registerable) {
	sqsH, ok1 := sqsReg.(*sqsbackend.Handler)
	cwH, ok2 := cwReg.(*cwbackend.Handler)

	if !ok1 || !ok2 {
		return
	}

	sqsBk, bk1Ok := sqsH.Backend.(*sqsbackend.InMemoryBackend)
	cwBk, bk2Ok := cwH.Backend.(*cwbackend.InMemoryBackend)

	if !bk1Ok || !bk2Ok {
		return
	}

	sqsBk.SetMetricEmitter(
		sqsbackend.MetricEmitterFunc(
			func(namespace, name string, value float64, unit string) error {
				err := cwBk.PutMetricData(namespace, []cwbackend.MetricDatum{
					{
						MetricName: name,
						Value:      value,
						Unit:       unit,
						Timestamp:  time.Now(),
					},
				})

				return err
			},
		),
	)
}

// wireEventBridgeDelivery connects EventBridge fan-out to Lambda, SQS, SNS, Kinesis Data Streams,
// Kinesis Data Firehose, ECS, Step Functions, CloudWatch Logs, and API Destination targets.
// ebReg, lambdaReg, sqsReg, snsReg, kinesisReg, firehoseReg, ecsReg, sfnReg, cwlogsReg must be the
// service.Registerable values returned by their respective providers.
func wireEventBridgeDelivery(
	ebReg, lambdaReg, sqsReg, snsReg, kinesisReg, firehoseReg, ecsReg, sfnReg, cwlogsReg service.Registerable,
) {
	ebH, ok := ebReg.(*ebbackend.Handler)
	if !ok {
		return
	}

	ebBk, bkOk := ebH.Backend.(*ebbackend.InMemoryBackend)
	if !bkOk {
		return
	}

	dt := &ebbackend.DeliveryTargets{}

	wireEventBridgeCoreTargets(dt, lambdaReg, sqsReg, snsReg)
	wireEventBridgeExtendedTargets(dt, kinesisReg, firehoseReg, ecsReg, sfnReg, cwlogsReg)

	// EventBridge itself resolves and rate-limits API destinations, so the backend
	// satisfies eventbridge.APIDestinationResolver directly.
	dt.APIDestinations = ebBk

	ebBk.SetDeliveryTargets(dt)
}

// wireEventBridgeCoreTargets populates the Lambda, SQS, and SNS delivery targets.
func wireEventBridgeCoreTargets(dt *ebbackend.DeliveryTargets, lambdaReg, sqsReg, snsReg service.Registerable) {
	if lambdaH, lambdaOk := lambdaReg.(*lambdabackend.Handler); lambdaOk {
		if lambdaBk, bkOk := lambdaH.Backend.(*lambdabackend.InMemoryBackend); bkOk {
			dt.Lambda = lambdaBk
		}
	}

	if sqsH, sqsOk := sqsReg.(*sqsbackend.Handler); sqsOk {
		if sqsBk, bkOk := sqsH.Backend.(*sqsbackend.InMemoryBackend); bkOk {
			dt.SQS = &sqsSenderAdapter{backend: sqsBk}
		}
	}

	if snsH, snsOk := snsReg.(*snsbackend.Handler); snsOk {
		if snsBk, bkOk := snsH.Backend.(*snsbackend.InMemoryBackend); bkOk {
			dt.SNS = &snsPublisherAdapter{backend: snsBk}
		}
	}
}

// wireEventBridgeExtendedTargets populates the Kinesis Data Streams, Kinesis Data Firehose,
// ECS, Step Functions, and CloudWatch Logs delivery targets.
func wireEventBridgeExtendedTargets(
	dt *ebbackend.DeliveryTargets, kinesisReg, firehoseReg, ecsReg, sfnReg, cwlogsReg service.Registerable,
) {
	if kinesisH, kinesisOk := kinesisReg.(*kinesisbackend.Handler); kinesisOk {
		if kinesisBk, bkOk := kinesisH.Backend.(*kinesisbackend.InMemoryBackend); bkOk {
			dt.KinesisStream = &ebKinesisStreamAdapter{backend: kinesisBk}
		}
	}

	if firehoseH, firehoseOk := firehoseReg.(*firehosebackend.Handler); firehoseOk {
		if firehoseBk, bkOk := firehoseH.Backend.(*firehosebackend.InMemoryBackend); bkOk {
			dt.KinesisFirehose = &ebFirehoseAdapter{backend: firehoseBk}
		}
	}

	if ecsH, ecsOk := ecsReg.(*ecsbackend.Handler); ecsOk {
		if ecsBk, bkOk := ecsH.Backend.(*ecsbackend.InMemoryBackend); bkOk {
			dt.ECS = &ebECSTaskRunnerAdapter{backend: ecsBk}
		}
	}

	if sfnH, sfnOk := sfnReg.(*sfnbackend.Handler); sfnOk {
		if sfnBk, bkOk := sfnH.Backend.(*sfnbackend.InMemoryBackend); bkOk {
			dt.StepFunctions = &sfnbackend.EBStartExecutionAdapter{B: sfnBk}
		}
	}

	if cwlogsH, cwlogsOk := cwlogsReg.(*cwlogsbackend.Handler); cwlogsOk {
		if cwlogsBk, bkOk := cwlogsH.Backend.(*cwlogsbackend.InMemoryBackend); bkOk {
			dt.CloudWatchLogs = &ebCloudWatchLogsAdapter{backend: cwlogsBk}
		}
	}
}

// sqsSenderAdapter adapts the SQS backend to the eventbridge.SQSSender interface.
type sqsSenderAdapter struct {
	backend *sqsbackend.InMemoryBackend
}

func (a *sqsSenderAdapter) SendMessageToQueue(
	_ context.Context,
	queueARN, messageBody string,
) error {
	// Convert SQS ARN to queue name (last segment after ':').
	queueURL := arnToSQSQueueURL(queueARN)
	_, err := a.backend.SendMessage(&sqsbackend.SendMessageInput{
		QueueURL:    queueURL,
		MessageBody: messageBody,
	})

	return err
}

// snsPublisherAdapter adapts the SNS backend to the eventbridge.SNSPublisher interface.
type snsPublisherAdapter struct {
	backend *snsbackend.InMemoryBackend
}

func (a *snsPublisherAdapter) PublishToTopic(_ context.Context, topicARN, message string) error {
	_, err := a.backend.Publish(topicARN, message, "", "", nil)

	return err
}

// ebKinesisStreamAdapter adapts the Kinesis backend to the eventbridge.KinesisStreamPublisher interface.
type ebKinesisStreamAdapter struct {
	backend *kinesisbackend.InMemoryBackend
}

func (a *ebKinesisStreamAdapter) PutRecord(ctx context.Context, streamARN, partitionKey, data string) error {
	// Convert Kinesis stream ARN to stream name (last segment after '/').
	parts := strings.Split(streamARN, "/")
	streamName := parts[len(parts)-1]

	_, err := a.backend.PutRecord(ctx, &kinesisbackend.PutRecordInput{
		StreamName:   streamName,
		PartitionKey: partitionKey,
		Data:         []byte(data),
	})

	return err
}

// ebFirehoseAdapter adapts the Firehose backend to the eventbridge.KinesisFirehosePublisher interface.
type ebFirehoseAdapter struct {
	backend *firehosebackend.InMemoryBackend
}

func (a *ebFirehoseAdapter) PutRecord(ctx context.Context, deliveryStreamARN, data string) error {
	// Convert Firehose delivery stream ARN to stream name (last segment after '/').
	parts := strings.Split(deliveryStreamARN, "/")
	streamName := parts[len(parts)-1]

	return a.backend.PutRecord(ctx, streamName, []byte(data))
}

// ebECSTaskRunnerAdapter adapts the ECS backend to the eventbridge.ECSTaskRunner
// and eventbridge.ECSTaskRunnerWithParams interfaces.
type ebECSTaskRunnerAdapter struct {
	backend *ecsbackend.InMemoryBackend
}

func (a *ebECSTaskRunnerAdapter) RunTask(ctx context.Context, clusterARN string, payload []byte) error {
	return a.RunTaskWithParams(ctx, clusterARN, nil, payload)
}

func convertEcsNetworkConfig(cfg *ebbackend.NetworkConfiguration) *ecsbackend.NetworkConfiguration {
	if cfg == nil || cfg.AwsvpcConfiguration == nil {
		return nil
	}

	return &ecsbackend.NetworkConfiguration{
		AwsvpcConfiguration: &ecsbackend.AwsvpcConfiguration{
			AssignPublicIP: cfg.AwsvpcConfiguration.AssignPublicIP,
			Subnets:        cfg.AwsvpcConfiguration.Subnets,
			SecurityGroups: cfg.AwsvpcConfiguration.SecurityGroups,
		},
	}
}

func convertEcsPlacementConstraints(pcs []ebbackend.PlacementConstraint) []ecsbackend.PlacementConstraint {
	if len(pcs) == 0 {
		return nil
	}

	out := make([]ecsbackend.PlacementConstraint, len(pcs))
	for i, pc := range pcs {
		out[i] = ecsbackend.PlacementConstraint{
			Type:       pc.Type,
			Expression: pc.Expression,
		}
	}

	return out
}

func convertEcsPlacementStrategy(pss []ebbackend.PlacementStrategy) []ecsbackend.PlacementStrategy {
	if len(pss) == 0 {
		return nil
	}

	out := make([]ecsbackend.PlacementStrategy, len(pss))
	for i, ps := range pss {
		out[i] = ecsbackend.PlacementStrategy{
			Type:  ps.Type,
			Field: ps.Field,
		}
	}

	return out
}

func convertEcsCapacityProviderStrategy(
	cps []ebbackend.CapacityProviderStrategyItem,
) []ecsbackend.CapacityProviderStrategyItem {
	if len(cps) == 0 {
		return nil
	}

	out := make([]ecsbackend.CapacityProviderStrategyItem, len(cps))
	for i, cp := range cps {
		out[i] = ecsbackend.CapacityProviderStrategyItem{
			CapacityProvider: cp.CapacityProvider,
			Base:             int(cp.Base),
			Weight:           int(cp.Weight),
		}
	}

	return out
}

func convertEcsTags(tags []ebbackend.EcsTag) []ecsbackend.Tag {
	if len(tags) == 0 {
		return nil
	}

	out := make([]ecsbackend.Tag, len(tags))
	for i, t := range tags {
		out[i] = ecsbackend.Tag{
			Key:   t.Key,
			Value: t.Value,
		}
	}

	return out
}

func buildECSRunInput(
	clusterARN string,
	params *ebbackend.EcsParameters,
	payload []byte,
) ecsbackend.RunTaskInput {
	var input map[string]any
	if len(payload) > 0 {
		_ = json.Unmarshal(payload, &input)
	}

	runInput := ecsbackend.RunTaskInput{
		Cluster: clusterARN,
	}

	if params != nil {
		runInput.TaskDefinition = params.TaskDefinitionArn
		runInput.LaunchType = params.LaunchType
		runInput.Group = params.Group
		runInput.PlatformVersion = params.PlatformVersion
		runInput.PropagateTags = params.PropagateTags
		runInput.EnableECSManagedTags = params.EnableECSManagedTags
		runInput.EnableExecuteCommand = params.EnableExecuteCommand
		runInput.Count = int(params.TaskCount)
		runInput.NetworkConfiguration = convertEcsNetworkConfig(params.NetworkConfiguration)
		runInput.PlacementConstraints = convertEcsPlacementConstraints(params.PlacementConstraints)
		runInput.PlacementStrategy = convertEcsPlacementStrategy(params.PlacementStrategy)
		runInput.CapacityProviderStrategy = convertEcsCapacityProviderStrategy(params.CapacityProviderStrategy)
		runInput.Tags = convertEcsTags(params.Tags)
	}

	if runInput.TaskDefinition == "" && input != nil {
		if td, ok := input["TaskDefinition"].(string); ok {
			runInput.TaskDefinition = td
		}
	}
	if runInput.LaunchType == "" && input != nil {
		if lt, ok := input["LaunchType"].(string); ok {
			runInput.LaunchType = lt
		}
	}

	return runInput
}

func (a *ebECSTaskRunnerAdapter) RunTaskWithParams(
	_ context.Context,
	clusterARN string,
	params *ebbackend.EcsParameters,
	payload []byte,
) error {
	runInput := buildECSRunInput(clusterARN, params, payload)
	_, err := a.backend.RunTask(runInput)

	return err
}

// ebCloudWatchLogsAdapter adapts the CloudWatch Logs backend to the
// eventbridge.CloudWatchLogsPublisher interface.
type ebCloudWatchLogsAdapter struct {
	backend *cwlogsbackend.InMemoryBackend
}

func (a *ebCloudWatchLogsAdapter) PutLogEvents(
	ctx context.Context,
	logGroupName, logStreamName string,
	logEvents []any,
) error {
	now := time.Now().UnixMilli()
	events := make([]cwlogsbackend.InputLogEvent, 0, len(logEvents))

	for _, e := range logEvents {
		message, _ := e.(string)
		events = append(events, cwlogsbackend.InputLogEvent{
			Message:   message,
			Timestamp: now,
		})
	}

	_, err := a.backend.PutLogEvents(ctx, logGroupName, logStreamName, "", events)

	return err
}

// wireS3Notifications connects the S3 handler to SQS, SNS, Lambda, and EventBridge backends so that
// bucket notification configurations are honoured on PutObject, CopyObject, DeleteObject, and CompleteMultipartUpload.
func wireS3Notifications(s3Reg, sqsReg, snsReg, lambdaReg, ebReg service.Registerable) {
	s3H, ok := s3Reg.(*s3backend.S3Handler)
	if !ok {
		return
	}

	targets := &s3backend.NotificationTargets{}

	if sqsH, sqsOk := sqsReg.(*sqsbackend.Handler); sqsOk {
		if sqsBk, bkOk := sqsH.Backend.(*sqsbackend.InMemoryBackend); bkOk {
			targets.SQSSender = &sqsSenderAdapter{backend: sqsBk}
		}
	}

	if snsH, snsOk := snsReg.(*snsbackend.Handler); snsOk {
		if snsBk, bkOk := snsH.Backend.(*snsbackend.InMemoryBackend); bkOk {
			targets.SNSPublisher = &s3SNSPublisherAdapter{backend: snsBk}
		}
	}

	if lambdaH, lambdaOk := lambdaReg.(*lambdabackend.Handler); lambdaOk {
		if lambdaBk, bkOk := lambdaH.Backend.(*lambdabackend.InMemoryBackend); bkOk {
			targets.LambdaInvoker = lambdaBk
		}
	}

	if ebH, ebOk := ebReg.(*ebbackend.Handler); ebOk {
		if ebBk, bkOk := ebH.Backend.(*ebbackend.InMemoryBackend); bkOk {
			targets.EventBridgePublisher = &s3EventBridgeAdapter{backend: ebBk}
		}
	}

	s3H.SetNotificationDispatcher(
		s3backend.NewNotificationDispatcher(targets, config.DefaultRegion),
	)
}

// s3SNSPublisherAdapter adapts the SNS backend to the s3.SNSPublisher interface.
type s3SNSPublisherAdapter struct {
	backend *snsbackend.InMemoryBackend
}

func (a *s3SNSPublisherAdapter) PublishToTopic(
	_ context.Context,
	topicARN, message, _ string,
) error {
	_, err := a.backend.Publish(topicARN, message, "", "", nil)

	return err
}

// s3EventBridgeAdapter adapts the EventBridge backend to the s3.EventBridgePublisher interface.
type s3EventBridgeAdapter struct {
	backend *ebbackend.InMemoryBackend
}

func (a *s3EventBridgeAdapter) PublishS3Event(
	ctx context.Context,
	source, detailType, detail string,
) {
	// Best-effort S3 -> EventBridge forwarding: PublishS3Event has no error
	// channel (s3.EventBridgePublisher is fire-and-forget), so the PutEvents
	// error (added by the Phase 3.3 store conversion) is explicitly discarded.
	_, _ = a.backend.PutEvents(ctx, []ebbackend.EventEntry{
		{Source: source, DetailType: detailType, Detail: detail},
	})
}

// wireSSMKMS connects the SSM backend to the KMS backend so that SecureString
// parameters whose KeyId is set are encrypted/decrypted using real KMS keys.
func wireSSMKMS(ssmReg, kmsReg service.Registerable) {
	ssmH, ok := ssmReg.(*ssmbackend.Handler)
	if !ok {
		return
	}
	ssmBk, ok := ssmH.Backend.(*ssmbackend.InMemoryBackend)
	if !ok {
		return
	}
	kmsH, ok := kmsReg.(*kmsbackend.Handler)
	if !ok {
		return
	}
	kmsBk, ok := kmsH.Backend.(*kmsbackend.InMemoryBackend)
	if !ok {
		return
	}
	ssmBk.WithKMS(&ssmKMSAdapter{backend: kmsBk})
}

// wireSSMParameterPolicyNotifications lets SSM emit EventBridge events when a
// parameter's ExpirationNotification or NoChangeNotification policy comes due.
// Without this the notifier stays nil and the janitor sweep is a no-op.
func wireSSMParameterPolicyNotifications(ssmReg, ebReg service.Registerable) {
	ssmH, ok := ssmReg.(*ssmbackend.Handler)
	if !ok {
		return
	}
	ssmBk, ok := ssmH.Backend.(*ssmbackend.InMemoryBackend)
	if !ok {
		return
	}
	ebH, ok := ebReg.(*ebbackend.Handler)
	if !ok {
		return
	}
	ebBk, ok := ebH.Backend.(*ebbackend.InMemoryBackend)
	if !ok {
		return
	}
	ssmBk.SetParameterPolicyNotifier(ebBk)
}

// ssmKMSAdapter adapts kms.InMemoryBackend to ssm.KMSEncryptor.
type ssmKMSAdapter struct {
	backend *kmsbackend.InMemoryBackend
}

func (a *ssmKMSAdapter) EncryptSSM(keyID string, plaintext []byte) ([]byte, error) {
	out, err := a.backend.Encrypt(context.Background(), &kmsbackend.EncryptInput{
		KeyID:     keyID,
		Plaintext: plaintext,
	})
	if err != nil {
		return nil, err
	}

	return out.CiphertextBlob, nil
}

func (a *ssmKMSAdapter) DecryptSSM(ciphertext []byte) ([]byte, error) {
	out, err := a.backend.Decrypt(context.Background(), &kmsbackend.DecryptInput{
		CiphertextBlob: ciphertext,
	})
	if err != nil {
		return nil, err
	}

	return out.Plaintext, nil
}

// wireKinesisKMS connects the Kinesis backend to the KMS backend so
// StartStreamEncryption's KeyId is validated against real KMS key state
// (KMSNotFoundException/KMSDisabledException/KMSInvalidStateException),
// mirroring wireSSMKMS above.
func wireKinesisKMS(kinesisReg, kmsReg service.Registerable) {
	kinesisH, ok := kinesisReg.(*kinesisbackend.Handler)
	if !ok {
		return
	}
	kinesisBk, ok := kinesisH.Backend.(*kinesisbackend.InMemoryBackend)
	if !ok {
		return
	}
	kmsH, ok := kmsReg.(*kmsbackend.Handler)
	if !ok {
		return
	}
	kmsBk, ok := kmsH.Backend.(*kmsbackend.InMemoryBackend)
	if !ok {
		return
	}
	kinesisBk.WithKMSValidator(&kinesisKMSAdapter{backend: kmsBk})
}

// kinesisKMSAdapter adapts kms.InMemoryBackend to kinesis.KMSKeyValidator.
type kinesisKMSAdapter struct {
	backend *kmsbackend.InMemoryBackend
}

func (a *kinesisKMSAdapter) ValidateKMSKey(ctx context.Context, keyID string) error {
	out, err := a.backend.DescribeKey(ctx, &kmsbackend.DescribeKeyInput{KeyID: keyID})
	if err != nil {
		// ErrKeyNotFound (nonexistent key/alias) and any other DescribeKey
		// failure (e.g. a malformed KeyId that slipped past kinesis's own
		// format check) both surface as "key not found" -- kinesis has no
		// finer-grained sentinel for "KMS backend rejected the lookup".
		return kinesisbackend.ErrKMSNotFound
	}

	switch out.KeyMetadata.KeyState {
	case kmsbackend.KeyStateEnabled:
		return nil
	case kmsbackend.KeyStateDisabled:
		return kinesisbackend.ErrKMSDisabled
	default:
		// PendingDeletion, PendingImport, or any other non-Enabled state.
		return kinesisbackend.ErrKMSInvalidState
	}
}

// wireSecretsManagerKMS connects the Secrets Manager backend to the KMS backend
// so that secret values are encrypted/decrypted using real KMS keys instead of
// being stored opaquely, mirroring wireSSMKMS above.
func wireSecretsManagerKMS(smReg, kmsReg service.Registerable) {
	smH, ok := smReg.(*secretsmanagerbackend.Handler)
	if !ok {
		return
	}

	smBk, ok := smH.Backend.(*secretsmanagerbackend.InMemoryBackend)
	if !ok {
		return
	}

	kmsH, ok := kmsReg.(*kmsbackend.Handler)
	if !ok {
		return
	}

	kmsBk, ok := kmsH.Backend.(*kmsbackend.InMemoryBackend)
	if !ok {
		return
	}

	smBk.SetKMSEncryptor(&secretsManagerKMSAdapter{backend: kmsBk})
}

// secretsManagerKMSAdapter adapts kms.InMemoryBackend to
// secretsmanager.KMSEncryptor.
type secretsManagerKMSAdapter struct {
	backend *kmsbackend.InMemoryBackend
	// defaultKeyID lazily caches a real KMS key ID created to back
	// secretsmanager.DefaultKMSKeyAlias. Secrets Manager encrypts under the
	// literal alias "alias/aws/secretsmanager" (its default managed key) when
	// a secret has no KmsKeyId, but kms.InMemoryBackend.CreateAlias rejects
	// any "alias/aws/" prefix (reserved for genuine AWS managed keys), so
	// that alias can never be registered through the real KMS API surface.
	// Substituting a dedicated backing key here reproduces the same
	// behaviour -- every secret without an explicit key shares one
	// account-level default key -- without touching services/kms.
	defaultKeyID string
	mu           sync.Mutex
}

func (a *secretsManagerKMSAdapter) Encrypt(ctx context.Context, keyID string, plaintext []byte) ([]byte, error) {
	resolvedKeyID, err := a.resolveKeyID(ctx, keyID)
	if err != nil {
		return nil, err
	}

	out, err := a.backend.Encrypt(ctx, &kmsbackend.EncryptInput{
		KeyID:     resolvedKeyID,
		Plaintext: plaintext,
	})
	if err != nil {
		return nil, err
	}

	return out.CiphertextBlob, nil
}

func (a *secretsManagerKMSAdapter) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	// No KeyId hint needed: the KMS backend derives the encrypting key from
	// the ciphertext blob itself.
	out, err := a.backend.Decrypt(ctx, &kmsbackend.DecryptInput{
		CiphertextBlob: ciphertext,
	})
	if err != nil {
		return nil, err
	}

	return out.Plaintext, nil
}

// resolveKeyID substitutes secretsmanager.DefaultKMSKeyAlias with a lazily
// created, cached backing key ID; any other key ID/ARN/alias (an explicit
// customer-managed key) passes through unchanged.
func (a *secretsManagerKMSAdapter) resolveKeyID(ctx context.Context, keyID string) (string, error) {
	if keyID != secretsmanagerbackend.DefaultKMSKeyAlias {
		return keyID, nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.defaultKeyID != "" {
		return a.defaultKeyID, nil
	}

	out, err := a.backend.CreateKey(ctx, &kmsbackend.CreateKeyInput{
		Description: "Default master key that protects my Secrets Manager secrets when no other key is defined",
	})
	if err != nil {
		return "", err
	}

	a.defaultKeyID = out.KeyMetadata.KeyID

	return a.defaultKeyID, nil
}

// wireAPIGatewayLambda connects the API Gateway handler to the Lambda backend
// for AWS_PROXY integrations.
func wireAPIGatewayLambda(apigwReg, apigwv2Reg, lambdaReg service.Registerable) {
	lambdaH, ok := lambdaReg.(*lambdabackend.Handler)
	if !ok {
		return
	}
	lambdaBk, ok := lambdaH.Backend.(*lambdabackend.InMemoryBackend)
	if !ok {
		return
	}

	if apigwH, ok2 := apigwReg.(*apigwbackend.Handler); ok2 {
		apigwH.SetLambdaInvoker(lambdaBk)
	}
	if apigwv2H, ok3 := apigwv2Reg.(*apigwv2backend.Handler); ok3 {
		apigwv2H.SetLambdaInvoker(lambdaBk)
	}
}

// wireAPIGatewayCognito wires the Cognito JWKS provider into both API Gateway handlers
// so that Cognito JWT authorizers verify token signatures rather than skipping them.
func wireAPIGatewayCognito(apigwReg, apigwv2Reg, cognitoReg service.Registerable) {
	cognitoH, ok := cognitoReg.(*cognitoidpbackend.Handler)
	if !ok {
		return
	}

	cognitoBk := cognitoH.Backend

	if apigwH, ok2 := apigwReg.(*apigwbackend.Handler); ok2 {
		apigwH.SetJWKSProvider(cognitoBk)
	}

	if apigwv2H, ok3 := apigwv2Reg.(*apigwv2backend.Handler); ok3 {
		apigwv2H.SetJWKSProvider(cognitoBk)
	}
}

// wireAPIGatewayManagementAPI connects the API Gateway V2 handler to the API Gateway Management API backend
// for WebSocket connection management.
func wireAPIGatewayManagementAPI(apigwv2Reg, mngtReg service.Registerable) {
	if apigwv2H, ok := apigwv2Reg.(*apigwv2backend.Handler); ok {
		if mngtH, mngtOk := mngtReg.(*apigwmgmtbackend.Handler); mngtOk {
			if mngtBk, bkOk := mngtH.Backend.(*apigwmgmtbackend.InMemoryBackend); bkOk {
				apigwv2H.SetManagementAPIBackend(mngtBk)
			}
		}
	}
}

// so that Task states with Lambda resources can invoke functions.
func wireStepFunctionsLambda(sfnReg, lambdaReg service.Registerable) {
	sfnH, ok := sfnReg.(*sfnbackend.Handler)
	if !ok {
		return
	}

	sfnBk, bkOk := sfnH.Backend.(*sfnbackend.InMemoryBackend)
	if !bkOk {
		return
	}

	if lambdaH, lambdaOk := lambdaReg.(*lambdabackend.Handler); lambdaOk {
		if lambdaBk, bk2Ok := lambdaH.Backend.(*lambdabackend.InMemoryBackend); bk2Ok {
			sfnBk.SetLambdaInvoker(lambdaBk)
		}
	}
}

// wireCognitoLambdaTriggers connects Cognito User Pool Lambda triggers to the
// Lambda backend so configured triggers (PreSignUp, PostConfirmation,
// PreTokenGeneration, CustomMessage) actually invoke their functions.
func wireCognitoLambdaTriggers(cognitoReg, lambdaReg service.Registerable) {
	cognitoH, ok := cognitoReg.(*cognitoidpbackend.Handler)
	if !ok {
		return
	}

	lambdaH, lambdaOk := lambdaReg.(*lambdabackend.Handler)
	if !lambdaOk {
		return
	}

	lambdaBk, bkOk := lambdaH.Backend.(*lambdabackend.InMemoryBackend)
	if !bkOk {
		return
	}

	cognitoH.Backend.SetLambdaTriggerInvoker(&cognitoLambdaTriggerAdapter{backend: lambdaBk})
}

// cognitoLambdaTriggerAdapter adapts the Lambda backend to the
// cognitoidp.LambdaTriggerInvoker interface, invoking a trigger function
// synchronously (RequestResponse) and round-tripping the JSON event envelope.
type cognitoLambdaTriggerAdapter struct {
	backend *lambdabackend.InMemoryBackend
}

func (a *cognitoLambdaTriggerAdapter) InvokeTrigger(
	ctx context.Context,
	functionARN string,
	event map[string]any,
) (map[string]any, error) {
	payload, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

	result, _, err := a.backend.InvokeFunction(
		ctx, functionARN, lambdabackend.InvocationTypeRequestResponse, payload,
	)
	if err != nil {
		return nil, err
	}

	var resp map[string]any
	if err = json.Unmarshal(result, &resp); err != nil {
		return nil, err
	}

	return resp, nil
}

// wireStepFunctionsServiceIntegrations connects the Step Functions backend to SQS, SNS, DynamoDB,
// ECS, Glue, EventBridge, and S3 backends so that Task states with service integration resources
// (and Distributed Map's S3 ItemReader) can invoke those services.
func wireStepFunctionsServiceIntegrations(
	sfnReg, sqsReg, snsReg, ddbReg, ecsReg, glueReg, ebReg, s3Reg service.Registerable,
) {
	sfnH, ok := sfnReg.(*sfnbackend.Handler)
	if !ok {
		return
	}

	sfnBk, bkOk := sfnH.Backend.(*sfnbackend.InMemoryBackend)
	if !bkOk {
		return
	}

	if sqsH, sqsOk := sqsReg.(*sqsbackend.Handler); sqsOk {
		sfnBk.SetSQSIntegration(sfnbackend.NewSQSIntegration(sqsH.Backend))
	}

	if snsH, snsOk := snsReg.(*snsbackend.Handler); snsOk {
		sfnBk.SetSNSIntegration(sfnbackend.NewSNSIntegration(snsH.Backend))
	}

	if ddbH, ddbOk := ddbReg.(*ddbbackend.DynamoDBHandler); ddbOk {
		sfnBk.SetDynamoDBIntegration(sfnbackend.NewDynamoDBIntegration(ddbH.Backend))
	}

	if ecsH, ecsOk := ecsReg.(*ecsbackend.Handler); ecsOk {
		if ecsBk, ecsBkOk := ecsH.Backend.(*ecsbackend.InMemoryBackend); ecsBkOk {
			sfnBk.SetECSIntegration(ecsBk)
		}
	}

	if glueH, glueOk := glueReg.(*gluebackend.Handler); glueOk {
		if glueBk, glueBkOk := glueH.Backend.(*gluebackend.InMemoryBackend); glueBkOk {
			sfnBk.SetGlueIntegration(glueBk)
		}
	}

	if ebH, ebOk := ebReg.(*ebbackend.Handler); ebOk {
		if ebBk, ebBkOk := ebH.Backend.(*ebbackend.InMemoryBackend); ebBkOk {
			sfnBk.SetEventBridgeIntegration(ebBk)
		}
	}

	if s3H, s3Ok := s3Reg.(*s3backend.S3Handler); s3Ok {
		sfnBk.SetS3Reader(sfnbackend.NewS3Integration(s3H.Backend))
		sfnBk.SetS3ResultWriter(sfnbackend.NewS3ResultWriterIntegration(s3H.Backend))
	}
}

// wireKinesisLambda connects the Kinesis backend to the Lambda event source poller
// so that records written to Kinesis streams trigger Lambda functions with active
// event source mappings.
func wireKinesisLambda(kinesisReg, lambdaReg service.Registerable) {
	kinesisH, ok := kinesisReg.(*kinesisbackend.Handler)
	if !ok {
		return
	}

	kinesisBk, bkOk := kinesisH.Backend.(*kinesisbackend.InMemoryBackend)
	if !bkOk {
		return
	}

	if lambdaH, lambdaOk := lambdaReg.(*lambdabackend.Handler); lambdaOk {
		if lambdaBk, bk2Ok := lambdaH.Backend.(*lambdabackend.InMemoryBackend); bk2Ok {
			adapter := &kinesisReaderAdapter{backend: kinesisBk}
			lambdaBk.SetKinesisPoller(lambdabackend.NewEventSourcePoller(lambdaBk, adapter))
		}
	}
}

// kinesisReaderAdapter adapts the Kinesis backend to the lambda.KinesisReader interface.
type kinesisReaderAdapter struct {
	backend *kinesisbackend.InMemoryBackend
}

func (a *kinesisReaderAdapter) GetShardIDs(streamName string) ([]string, error) {
	out, err := a.backend.DescribeStream(
		context.Background(),
		&kinesisbackend.DescribeStreamInput{StreamName: streamName},
	)
	if err != nil {
		return nil, err
	}

	ids := make([]string, len(out.Shards))
	for i, s := range out.Shards {
		ids[i] = s.ShardID
	}

	return ids, nil
}

func (a *kinesisReaderAdapter) GetShardIterator(
	streamName, shardID, iteratorType, startingSeqNum string,
) (string, error) {
	out, err := a.backend.GetShardIterator(context.Background(), &kinesisbackend.GetShardIteratorInput{
		StreamName:             streamName,
		ShardID:                shardID,
		ShardIteratorType:      iteratorType,
		StartingSequenceNumber: startingSeqNum,
	})
	if err != nil {
		return "", err
	}

	return out.ShardIterator, nil
}

func (a *kinesisReaderAdapter) GetRecords(
	iteratorToken string,
	limit int,
) ([]lambdabackend.KinesisRecord, string, error) {
	out, err := a.backend.GetRecords(context.Background(), &kinesisbackend.GetRecordsInput{
		ShardIterator: iteratorToken,
		Limit:         limit,
	})
	if err != nil {
		return nil, "", err
	}

	records := make([]lambdabackend.KinesisRecord, len(out.Records))
	for i, r := range out.Records {
		records[i] = lambdabackend.KinesisRecord{
			PartitionKey:   r.PartitionKey,
			SequenceNumber: r.SequenceNumber,
			Data:           r.Data,
			ArrivalTime:    r.ApproximateArrivalTimestamp,
		}
	}

	return records, out.NextShardIterator, nil
}

// ARN format: arn:aws:sqs:region:accountId:queueName
// URL format expected by SQS backend: http://endpoint/accountId/queueName
func arnToSQSQueueURL(arn string) string {
	parts := strings.Split(arn, ":")
	// Minimum parts for a valid SQS ARN: arn, aws, sqs, region, accountId, queueName
	const minARNParts = 6
	if len(parts) < minARNParts {
		return arn
	}

	accountID := parts[4]
	queueName := parts[5]

	return "http://local/" + accountID + "/" + queueName
}

// wireSQSLambda connects the SQS backend to the Lambda event source poller so
// that messages enqueued in SQS queues trigger Lambda functions with active
// SQS event source mappings.
func wireSQSLambda(sqsReg, lambdaReg service.Registerable) {
	sqsH, ok := sqsReg.(*sqsbackend.Handler)
	if !ok {
		return
	}

	sqsBk, bkOk := sqsH.Backend.(*sqsbackend.InMemoryBackend)
	if !bkOk {
		return
	}

	if lambdaH, lambdaOk := lambdaReg.(*lambdabackend.Handler); lambdaOk {
		if lambdaBk, bk2Ok := lambdaH.Backend.(*lambdabackend.InMemoryBackend); bk2Ok {
			lambdaBk.SetSQSReader(&sqsReaderAdapter{backend: sqsBk})
		}
	}
}

// wireLambdaAsyncDestinations connects the Lambda backend to the SQS, SNS, and
// Lambda backends so that async-invocation DeadLetterConfig and DestinationConfig
// (OnSuccess/OnFailure) outcomes are actually delivered to their target ARNs.
func wireLambdaAsyncDestinations(lambdaReg, sqsReg, snsReg service.Registerable) {
	lambdaH, ok := lambdaReg.(*lambdabackend.Handler)
	if !ok {
		return
	}

	lambdaBk, bkOk := lambdaH.Backend.(*lambdabackend.InMemoryBackend)
	if !bkOk {
		return
	}

	adapter := &lambdaAsyncDeliveryAdapter{lambda: lambdaBk}

	if sqsH, sqsOk := sqsReg.(*sqsbackend.Handler); sqsOk {
		if sqsBk, bk2Ok := sqsH.Backend.(*sqsbackend.InMemoryBackend); bk2Ok {
			adapter.sqs = sqsBk
		}
	}

	if snsH, snsOk := snsReg.(*snsbackend.Handler); snsOk {
		if snsBk, bk2Ok := snsH.Backend.(*snsbackend.InMemoryBackend); bk2Ok {
			adapter.sns = snsBk
		}
	}

	lambdaBk.SetAsyncDestinationDelivery(adapter)
}

// lambdaAsyncDeliveryAdapter routes an async-invocation outcome to its target ARN,
// dispatching by the ARN's service (SQS queue, SNS topic, or Lambda function).
type lambdaAsyncDeliveryAdapter struct {
	sqs    *sqsbackend.InMemoryBackend
	sns    *snsbackend.InMemoryBackend
	lambda *lambdabackend.InMemoryBackend
}

func (a *lambdaAsyncDeliveryAdapter) DeliverToTarget(
	ctx context.Context,
	targetARN string,
	payload []byte,
	attributes map[string]string,
) error {
	switch {
	case strings.HasPrefix(targetARN, "arn:aws:sqs:"):
		if a.sqs == nil {
			return nil
		}

		attrs := make(map[string]sqsbackend.MessageAttributeValue, len(attributes))
		for k, v := range attributes {
			attrs[k] = sqsbackend.MessageAttributeValue{DataType: "String", StringValue: v}
		}

		_, err := a.sqs.SendMessage(&sqsbackend.SendMessageInput{
			QueueURL:          arnToSQSQueueURL(targetARN),
			MessageBody:       string(payload),
			MessageAttributes: attrs,
		})

		return err
	case strings.HasPrefix(targetARN, "arn:aws:sns:"):
		if a.sns == nil {
			return nil
		}

		_, err := a.sns.Publish(targetARN, string(payload), "", "", nil)

		return err
	case strings.HasPrefix(targetARN, "arn:aws:lambda:"):
		if a.lambda == nil {
			return nil
		}

		fnName := targetARN[strings.LastIndex(targetARN, ":")+1:]
		_, _, err := a.lambda.InvokeFunction(ctx, fnName, lambdabackend.InvocationTypeEvent, payload)

		return err
	default:
		return nil
	}
}

// sqsReaderAdapter adapts the SQS InMemoryBackend to the lambda.SQSReader interface.
type sqsReaderAdapter struct {
	backend *sqsbackend.InMemoryBackend
}

func (a *sqsReaderAdapter) ReceiveMessagesLocal(
	queueARN string,
	maxMessages int,
) ([]*lambdabackend.SQSMessage, error) {
	url := arnToSQSQueueURL(queueARN)

	msgs, err := a.backend.ReceiveMessagesLocal(url, maxMessages)
	if err != nil {
		return nil, err
	}

	result := make([]*lambdabackend.SQSMessage, len(msgs))
	for i, m := range msgs {
		var msgAttrs map[string]lambdabackend.SQSMessageAttribute
		if len(m.MessageAttributes) > 0 {
			msgAttrs = make(map[string]lambdabackend.SQSMessageAttribute, len(m.MessageAttributes))
			for k, v := range m.MessageAttributes {
				msgAttrs[k] = lambdabackend.SQSMessageAttribute{
					DataType:    v.DataType,
					StringValue: v.StringValue,
					BinaryValue: v.BinaryValue,
				}
			}
		}

		result[i] = &lambdabackend.SQSMessage{
			MessageID:              m.MessageID,
			ReceiptHandle:          m.ReceiptHandle,
			Body:                   m.Body,
			Attributes:             m.Attributes,
			MessageAttributes:      msgAttrs,
			MD5OfBody:              m.MD5OfBody,
			MD5OfMessageAttributes: m.MD5OfMessageAttributes,
			SentTimestampMillis:    m.SentTimestamp,
		}
	}

	return result, nil
}

func (a *sqsReaderAdapter) DeleteMessagesLocal(queueARN string, receiptHandles []string) error {
	url := arnToSQSQueueURL(queueARN)

	return a.backend.DeleteMessagesLocal(url, receiptHandles)
}

// wireDynamoDBStreamLambda connects the DynamoDB Streams backend to the Lambda event source
// poller so that stream records trigger Lambda functions with active DynamoDB ESMs.
func wireDynamoDBStreamLambda(ddbReg, lambdaReg service.Registerable) {
	ddbH, ok := ddbReg.(*ddbbackend.DynamoDBHandler)
	if !ok {
		return
	}

	ddbBk, bkOk := ddbH.Backend.(*ddbbackend.InMemoryDB)
	if !bkOk {
		return
	}

	if lambdaH, lambdaOk := lambdaReg.(*lambdabackend.Handler); lambdaOk {
		if lambdaBk, bk2Ok := lambdaH.Backend.(*lambdabackend.InMemoryBackend); bk2Ok {
			lambdaBk.SetDynamoDBStreamsReader(&ddbStreamsReaderAdapter{backend: ddbBk})
		}
	}
}

// ddbStreamsReaderAdapter adapts the DynamoDB InMemoryDB to the lambda.DynamoDBStreamsReader interface.
type ddbStreamsReaderAdapter struct {
	backend *ddbbackend.InMemoryDB
}

func (a *ddbStreamsReaderAdapter) DescribeStreamShards(streamARN string) ([]string, error) {
	out, err := a.backend.DescribeStream(
		context.Background(),
		&awsddbstreams.DescribeStreamInput{StreamArn: aws.String(streamARN)},
	)
	if err != nil {
		return nil, err
	}

	if out.StreamDescription == nil {
		return nil, nil
	}

	shardIDs := make([]string, 0, len(out.StreamDescription.Shards))
	for _, s := range out.StreamDescription.Shards {
		if s.ShardId != nil {
			shardIDs = append(shardIDs, *s.ShardId)
		}
	}

	return shardIDs, nil
}

func (a *ddbStreamsReaderAdapter) GetStreamShardIterator(
	streamARN, shardID, iteratorType string,
) (string, error) {
	out, err := a.backend.GetShardIterator(
		context.Background(),
		&awsddbstreams.GetShardIteratorInput{
			StreamArn:         aws.String(streamARN),
			ShardId:           aws.String(shardID),
			ShardIteratorType: ddbstreamstypes.ShardIteratorType(iteratorType),
		},
	)
	if err != nil {
		return "", err
	}

	return aws.ToString(out.ShardIterator), nil
}

func (a *ddbStreamsReaderAdapter) GetStreamRecords(
	iteratorToken string,
	limit int,
) ([]lambdabackend.DynamoDBStreamRecord, string, error) {
	// Clamp limit to a valid int32 range. BatchSize is bounded by the Lambda ESM (max 10 000),
	// but we guard defensively against any caller passing a value outside int32 range.
	const maxStreamRecordsLimit = math.MaxInt32
	if limit <= 0 || limit > maxStreamRecordsLimit {
		limit = maxStreamRecordsLimit
	}

	lim := int32(math.MaxInt32)
	if limit > 0 && limit <= maxStreamRecordsLimit {
		lim = int32(limit)
	}
	out, err := a.backend.GetRecords(context.Background(), &awsddbstreams.GetRecordsInput{
		ShardIterator: aws.String(iteratorToken),
		Limit:         &lim,
	})
	if err != nil {
		return nil, "", err
	}

	records := make([]lambdabackend.DynamoDBStreamRecord, 0, len(out.Records))

	for _, r := range out.Records {
		rec := lambdabackend.DynamoDBStreamRecord{
			EventID:   aws.ToString(r.EventID),
			EventName: string(r.EventName),
		}

		populateDDBStreamRecord(&rec, r.Dynamodb)
		records = append(records, rec)
	}

	return records, aws.ToString(out.NextShardIterator), nil
}

// populateDDBStreamRecord fills in the DynamoDB-specific fields of a DynamoDBStreamRecord
// from the SDK StreamRecord payload.
func populateDDBStreamRecord(
	rec *lambdabackend.DynamoDBStreamRecord,
	ddb *ddbstreamstypes.StreamRecord,
) {
	if ddb == nil {
		return
	}

	rec.SequenceNumber = aws.ToString(ddb.SequenceNumber)
	rec.StreamViewType = string(ddb.StreamViewType)

	if ddb.SizeBytes != nil {
		rec.SizeBytes = *ddb.SizeBytes
	}

	if ddb.ApproximateCreationDateTime != nil {
		rec.ApproximateCreationDateTime = float64(ddb.ApproximateCreationDateTime.Unix())
	}

	if ddb.NewImage != nil {
		rec.NewImage = sdkDDBStreamItemToWire(ddb.NewImage)
	}

	if ddb.OldImage != nil {
		rec.OldImage = sdkDDBStreamItemToWire(ddb.OldImage)
	}

	if ddb.Keys != nil {
		rec.Keys = sdkDDBStreamItemToWire(ddb.Keys)
	}
}

// sdkDDBStreamItemToWire converts a DynamoDB Streams SDK attribute map to DynamoDB JSON wire format.
func sdkDDBStreamItemToWire(item map[string]ddbstreamstypes.AttributeValue) map[string]any {
	out := make(map[string]any, len(item))

	for k, v := range item {
		if w := sdkDDBStreamAttrToWire(v); w != nil {
			out[k] = w
		}
	}

	return out
}

// sdkDDBStreamAttrToWire converts a single DynamoDB Streams SDK attribute value to DynamoDB JSON
// wire format (map[string]any). Unknown attribute types are returned as nil and are excluded by
// the caller.
func sdkDDBStreamAttrToWire(av ddbstreamstypes.AttributeValue) map[string]any {
	switch v := av.(type) {
	case *ddbstreamstypes.AttributeValueMemberS:
		return map[string]any{"S": v.Value}
	case *ddbstreamstypes.AttributeValueMemberN:
		return map[string]any{"N": v.Value}
	case *ddbstreamstypes.AttributeValueMemberBOOL:
		return map[string]any{"BOOL": v.Value}
	case *ddbstreamstypes.AttributeValueMemberNULL:
		return map[string]any{"NULL": v.Value}
	case *ddbstreamstypes.AttributeValueMemberB:
		return map[string]any{"B": v.Value}
	case *ddbstreamstypes.AttributeValueMemberSS:
		return map[string]any{"SS": v.Value}
	case *ddbstreamstypes.AttributeValueMemberNS:
		return map[string]any{"NS": v.Value}
	case *ddbstreamstypes.AttributeValueMemberBS:
		return map[string]any{"BS": v.Value}
	case *ddbstreamstypes.AttributeValueMemberM:
		return map[string]any{"M": sdkDDBStreamItemToWire(v.Value)}
	case *ddbstreamstypes.AttributeValueMemberL:
		items := make([]any, len(v.Value))
		for i, elem := range v.Value {
			items[i] = sdkDDBStreamAttrToWire(elem)
		}

		return map[string]any{"L": items}
	}

	return nil
}

// wireCloudWatchAlarmActions connects the CloudWatch backend to SNS and Lambda so that
// alarm state transitions trigger action notifications.
func wireCloudWatchAlarmActions(cwReg, snsReg, lambdaReg service.Registerable) {
	cwH, ok1 := cwReg.(*cwbackend.Handler)
	snsH, ok2 := snsReg.(*snsbackend.Handler)
	lambdaH, ok3 := lambdaReg.(*lambdabackend.Handler)

	if !ok1 {
		return
	}

	cwBk, ok4 := cwH.Backend.(*cwbackend.InMemoryBackend)
	if !ok4 {
		return
	}

	if ok2 {
		if snsBk, isSNS := snsH.Backend.(*snsbackend.InMemoryBackend); isSNS {
			cwBk.SetSNSPublisher(&cwSNSPublisherAdapter{backend: snsBk})
		}
	}

	if ok3 {
		if lambdaBk, isLambda := lambdaH.Backend.(*lambdabackend.InMemoryBackend); isLambda {
			cwBk.SetLambdaInvoker(&cwLambdaInvokerAdapter{backend: lambdaBk})
		}
	}
}

// wireCloudWatchInfraActions connects the CloudWatch backend to the EC2 and Auto
// Scaling backends so that arn:aws:automate EC2 alarm actions and scaling-policy
// alarm actions actually mutate instance state / trigger scaling.
func wireCloudWatchInfraActions(cwReg, ec2Reg, asgReg service.Registerable) {
	cwH, ok := cwReg.(*cwbackend.Handler)
	if !ok {
		return
	}

	cwBk, ok := cwH.Backend.(*cwbackend.InMemoryBackend)
	if !ok {
		return
	}

	if ec2H, okEC2 := ec2Reg.(*ec2backend.Handler); okEC2 {
		if ec2Bk, isEC2 := ec2H.Backend.(*ec2backend.InMemoryBackend); isEC2 {
			cwBk.SetEC2Actioner(&cwEC2ActionerAdapter{backend: ec2Bk})
		}
	}

	if asgH, okASG := asgReg.(*autoscalingbackend.Handler); okASG {
		if asgBk, isASG := asgH.Backend.(*autoscalingbackend.InMemoryBackend); isASG {
			cwBk.SetAutoScalingExecutor(&cwAutoScalingAdapter{backend: asgBk})
		}
	}
}

// cwEC2ActionerAdapter adapts the EC2 backend to the cloudwatch.EC2InstanceActioner interface.
type cwEC2ActionerAdapter struct {
	backend *ec2backend.InMemoryBackend
}

func (a *cwEC2ActionerAdapter) StopInstances(ids []string) error {
	_, err := a.backend.StopInstances(ids)

	return err
}

func (a *cwEC2ActionerAdapter) TerminateInstances(ids []string) error {
	_, err := a.backend.TerminateInstances(ids)

	return err
}

func (a *cwEC2ActionerAdapter) RebootInstances(ids []string) error {
	return a.backend.RebootInstances(ids)
}

// cwAutoScalingAdapter adapts the Auto Scaling backend to the
// cloudwatch.AutoScalingPolicyExecutor interface.
type cwAutoScalingAdapter struct {
	backend *autoscalingbackend.InMemoryBackend
}

func (a *cwAutoScalingAdapter) ExecuteScalingPolicy(asgName, policyName string) error {
	return a.backend.ExecutePolicy(autoscalingbackend.ExecutePolicyInput{
		AutoScalingGroupName: asgName,
		PolicyName:           policyName,
	})
}

// wireAutoScalingEC2 wires Auto Scaling to the EC2 backend so scale-out
// launches real (mock) EC2 instances and scale-in terminates them there too,
// keeping EC2 DescribeInstances consistent with Auto Scaling group
// membership instead of Auto Scaling fabricating instance IDs with no EC2
// backing.
func wireAutoScalingEC2(asgReg, ec2Reg service.Registerable) {
	asgH, ok := asgReg.(*autoscalingbackend.Handler)
	if !ok {
		return
	}

	asgBk, ok := asgH.Backend.(*autoscalingbackend.InMemoryBackend)
	if !ok {
		return
	}

	ec2H, ok := ec2Reg.(*ec2backend.Handler)
	if !ok {
		return
	}

	ec2Bk, ok := ec2H.Backend.(*ec2backend.InMemoryBackend)
	if !ok {
		return
	}

	asgBk.SetEC2Launcher(&ec2AutoScalingLauncherAdapter{backend: ec2Bk})
}

// elbv2TargetRegistrarAdapter holds the ELBv2 backend and target-port
// resolution logic shared by the Auto Scaling and ECS ELBv2 registrar
// adapters below. Both need to translate a package-local ELBTarget (ID +
// optional port) into an elbv2.Target, defaulting an omitted (zero) port to
// the target group's own configured port: the elbv2 backend's
// RegisterTargets/DeregisterTargets methods, unlike its HTTP handlers (see
// defaultTargetPorts in services/elbv2/handler.go), do not default the port
// themselves, and these adapters call the backend methods directly.
type elbv2TargetRegistrarAdapter struct {
	backend *elbv2backend.InMemoryBackend
}

func (a *elbv2TargetRegistrarAdapter) port(tgArn string, targetPort int) int32 {
	if targetPort != 0 {
		return int32(targetPort) //nolint:gosec // container/instance ports fit in int32
	}

	tgs, err := a.backend.DescribeTargetGroups([]string{tgArn}, nil, "")
	if err != nil || len(tgs) == 0 {
		return 0
	}

	return tgs[0].Port
}

// wireAutoScalingELBv2 wires Auto Scaling to the ELBv2 backend so instances
// added to or removed from a group's TargetGroupARNs (via scale-out/in,
// TerminateInstanceInAutoScalingGroup, Attach/DetachInstances, and
// Attach/DetachLoadBalancerTargetGroups) register/deregister as real ELBv2
// targets, instead of TargetGroupARNs being stored and echoed with no effect
// on ELBv2 DescribeTargetHealth.
func wireAutoScalingELBv2(asgReg, elbv2Reg service.Registerable) {
	asgH, ok := asgReg.(*autoscalingbackend.Handler)
	if !ok {
		return
	}

	asgBk, ok := asgH.Backend.(*autoscalingbackend.InMemoryBackend)
	if !ok {
		return
	}

	elbv2H, ok := elbv2Reg.(*elbv2backend.Handler)
	if !ok {
		return
	}

	elbv2Bk, ok := elbv2H.Backend.(*elbv2backend.InMemoryBackend)
	if !ok {
		return
	}

	asgBk.SetELBv2Registrar(&autoscalingELBv2RegistrarAdapter{
		elbv2TargetRegistrarAdapter{backend: elbv2Bk},
	})
}

// autoscalingELBv2RegistrarAdapter adapts the ELBv2 backend to the
// autoscaling.ELBv2TargetRegistrar interface.
type autoscalingELBv2RegistrarAdapter struct {
	elbv2TargetRegistrarAdapter
}

func (a *autoscalingELBv2RegistrarAdapter) RegisterTargets(
	_ context.Context, tgArn string, targets []autoscalingbackend.ELBTarget,
) error {
	return a.backend.RegisterTargets(tgArn, a.toELBv2Targets(tgArn, targets))
}

func (a *autoscalingELBv2RegistrarAdapter) DeregisterTargets(
	_ context.Context, tgArn string, targets []autoscalingbackend.ELBTarget,
) error {
	return a.backend.DeregisterTargets(tgArn, a.toELBv2Targets(tgArn, targets))
}

func (a *autoscalingELBv2RegistrarAdapter) toELBv2Targets(
	tgArn string, targets []autoscalingbackend.ELBTarget,
) []elbv2backend.Target {
	out := make([]elbv2backend.Target, len(targets))
	for i, t := range targets {
		out[i] = elbv2backend.Target{ID: t.ID, Port: a.port(tgArn, t.Port)}
	}

	return out
}

// wireECSELBv2 wires ECS to the ELBv2 backend so tasks belonging to a
// service with LoadBalancers configured register/deregister as real ELBv2
// targets when they reach/leave RUNNING, instead of Service.LoadBalancers
// being stored and echoed with no effect on ELBv2 DescribeTargetHealth. Only
// awsvpc-mode (Fargate) tasks resolve to a usable target identity (their ENI
// private IP) — see ecs.privateIPFromAttachments; EC2-launch-type tasks are
// skipped rather than registered with a fabricated identity.
func wireECSELBv2(ecsReg, elbv2Reg service.Registerable) {
	ecsH, ok := ecsReg.(*ecsbackend.Handler)
	if !ok {
		return
	}

	ecsBk, ok := ecsH.Backend.(*ecsbackend.InMemoryBackend)
	if !ok {
		return
	}

	elbv2H, ok := elbv2Reg.(*elbv2backend.Handler)
	if !ok {
		return
	}

	elbv2Bk, ok := elbv2H.Backend.(*elbv2backend.InMemoryBackend)
	if !ok {
		return
	}

	ecsBk.SetELBv2Registrar(&ecsELBv2RegistrarAdapter{
		elbv2TargetRegistrarAdapter{backend: elbv2Bk},
	})
}

// ecsELBv2RegistrarAdapter adapts the ELBv2 backend to the
// ecs.ELBv2TargetRegistrar interface.
type ecsELBv2RegistrarAdapter struct {
	elbv2TargetRegistrarAdapter
}

func (a *ecsELBv2RegistrarAdapter) RegisterTargets(
	_ context.Context, tgArn string, targets []ecsbackend.ELBTarget,
) error {
	return a.backend.RegisterTargets(tgArn, a.toELBv2Targets(tgArn, targets))
}

func (a *ecsELBv2RegistrarAdapter) DeregisterTargets(
	_ context.Context, tgArn string, targets []ecsbackend.ELBTarget,
) error {
	return a.backend.DeregisterTargets(tgArn, a.toELBv2Targets(tgArn, targets))
}

func (a *ecsELBv2RegistrarAdapter) toELBv2Targets(
	tgArn string, targets []ecsbackend.ELBTarget,
) []elbv2backend.Target {
	out := make([]elbv2backend.Target, len(targets))
	for i, t := range targets {
		out[i] = elbv2backend.Target{ID: t.ID, Port: a.port(tgArn, t.Port)}
	}

	return out
}

// ec2AutoScalingLauncherAdapter adapts the EC2 backend to the
// autoscaling.EC2Launcher interface, translating an
// autoscaling.InstanceLaunchSpec into an EC2 RunInstances call (and tagging
// the results) and an autoscaling instance-ID list into an EC2
// TerminateInstances call.
type ec2AutoScalingLauncherAdapter struct {
	backend *ec2backend.InMemoryBackend
}

func (a *ec2AutoScalingLauncherAdapter) LaunchInstances(
	_ context.Context, spec autoscalingbackend.InstanceLaunchSpec, count int,
) ([]string, error) {
	instances, err := a.backend.RunInstances(spec.ImageID, spec.InstanceType, spec.SubnetID, count)
	if err != nil {
		return nil, err
	}

	ids := make([]string, len(instances))
	for i, inst := range instances {
		ids[i] = inst.ID
		if spec.KeyName != "" || len(spec.SecurityGroups) > 0 {
			if cfgErr := a.backend.SetInstanceLaunchConfig(inst.ID, spec.KeyName, spec.SecurityGroups); cfgErr != nil {
				return ids, fmt.Errorf("setting launch config for instance %s: %w", inst.ID, cfgErr)
			}
		}
	}

	if len(spec.Tags) > 0 {
		if tagErr := a.backend.CreateTags(ids, spec.Tags); tagErr != nil {
			return ids, tagErr
		}
	}

	return ids, nil
}

func (a *ec2AutoScalingLauncherAdapter) TerminateInstances(_ context.Context, ids []string) error {
	_, err := a.backend.TerminateInstances(ids)

	return err
}

func (a *ec2AutoScalingLauncherAdapter) ResolveLaunchTemplate(
	_ context.Context, id, name, version string,
) (string, string, error) {
	idOrName := id
	if idOrName == "" {
		idOrName = name
	}

	lt, err := a.backend.GetLaunchTemplate(idOrName, version)
	if err != nil {
		return "", "", err
	}

	return lt.ImageID, lt.InstanceType, nil
}

// cwSNSPublisherAdapter adapts the SNS backend to the cloudwatch.SNSPublisher interface.
type cwSNSPublisherAdapter struct {
	backend *snsbackend.InMemoryBackend
}

func (a *cwSNSPublisherAdapter) PublishToTopic(topicARN, message string) error {
	_, err := a.backend.Publish(topicARN, message, "CloudWatch Alarm", "", nil)

	return err
}

// cwLambdaInvokerAdapter adapts the Lambda backend to the cloudwatch.LambdaInvoker interface.
type cwLambdaInvokerAdapter struct {
	backend *lambdabackend.InMemoryBackend
}

func (a *cwLambdaInvokerAdapter) InvokeFunction(
	ctx context.Context,
	name string,
	_ string,
	payload []byte,
) ([]byte, int, error) {
	return a.backend.InvokeFunction(ctx, name, lambdabackend.InvocationTypeEvent, payload)
}

// wireLambdaCWLogs connects the Lambda backend to CloudWatch Logs so that
// function invocations produce log entries in /aws/lambda/{function-name}.
func wireLambdaCWLogs(lambdaReg, cwlogsReg service.Registerable) {
	lambdaH, ok := lambdaReg.(*lambdabackend.Handler)
	if !ok {
		return
	}

	lambdaBk, bkOk := lambdaH.Backend.(*lambdabackend.InMemoryBackend)
	if !bkOk {
		return
	}

	if cwlogsH, cwlogsOk := cwlogsReg.(*cwlogsbackend.Handler); cwlogsOk {
		if cwlogsBk, cwBkOk := cwlogsH.Backend.(*cwlogsbackend.InMemoryBackend); cwBkOk {
			lambdaBk.SetCWLogsBackend(&cwLogsAdapter{backend: cwlogsBk})
		}
	}
}

// wireTimestreamQueryTags connects the Timestream Query backend to
// Timestream Write's shared tag store (see timestreamquery.TagWriteBackend).
// timestreamwrite.InMemoryBackend.TagResource already matches the seam's
// interface signature, so no adapter type is needed.
func wireTimestreamQueryTags(tsqReg, tswReg service.Registerable) {
	tsqH, ok := tsqReg.(*timestreamquerybackend.Handler)
	if !ok {
		return
	}

	tsqBk, bkOk := tsqH.Backend.(*timestreamquerybackend.InMemoryBackend)
	if !bkOk {
		return
	}

	if tswH, tswOk := tswReg.(*timestreamwritebackend.Handler); tswOk {
		tsqBk.SetTagWriteBackend(tswH.Backend)
	}
}

// cwLogsAdapter adapts the CloudWatch Logs InMemoryBackend to the lambda.CWLogsBackend interface.
type cwLogsAdapter struct {
	backend *cwlogsbackend.InMemoryBackend
}

func (a *cwLogsAdapter) EnsureLogGroupAndStream(groupName, streamName string) error {
	if _, err := a.backend.CreateLogGroup(context.Background(), groupName, "", ""); err != nil &&
		!errors.Is(err, cwlogsbackend.ErrLogGroupAlreadyExists) {
		return err
	}

	if _, err := a.backend.CreateLogStream(context.Background(), groupName, streamName); err != nil &&
		!errors.Is(err, cwlogsbackend.ErrLogStreamAlreadyExist) {
		return err
	}

	return nil
}

func (a *cwLogsAdapter) PutLogLines(groupName, streamName string, messages []string) error {
	events := make([]cwlogsbackend.InputLogEvent, len(messages))
	now := time.Now().UnixMilli()

	for i, msg := range messages {
		events[i] = cwlogsbackend.InputLogEvent{Message: msg, Timestamp: now}
	}

	_, err := a.backend.PutLogEvents(context.Background(), groupName, streamName, "", events)

	return err
}

// wireCWLogsSubscriptionFilters wires the CloudWatch Logs subscription filter delivery
// to Lambda, Kinesis, and Firehose backends.
func wireCWLogsSubscriptionFilters(
	ctx context.Context,
	cwlogsReg, lambdaReg, kinesisReg, firehoseReg service.Registerable,
) {
	cwlogsH, ok := cwlogsReg.(*cwlogsbackend.Handler)
	if !ok {
		return
	}

	cwlogsBk, bkOk := cwlogsH.Backend.(*cwlogsbackend.InMemoryBackend)
	if !bkOk {
		return
	}

	d := &cwlogsSubscriptionDeliverer{}

	if lambdaH, lambdaOk := lambdaReg.(*lambdabackend.Handler); lambdaOk {
		if lambdaBk, bk2Ok := lambdaH.Backend.(*lambdabackend.InMemoryBackend); bk2Ok {
			d.lambda = lambdaBk
		}
	}

	if kinesisH, kinesisOk := kinesisReg.(*kinesisbackend.Handler); kinesisOk {
		if kinesisBk, bk2Ok := kinesisH.Backend.(*kinesisbackend.InMemoryBackend); bk2Ok {
			d.kinesis = kinesisBk
		}
	}

	if firehoseH, firehoseOk := firehoseReg.(*firehosebackend.Handler); firehoseOk {
		if fhBk, fhBkOk := firehoseH.Backend.(*firehosebackend.InMemoryBackend); fhBkOk {
			d.firehose = fhBk
		} else {
			logger.Load(ctx).
				WarnContext(ctx, "cwlogs: firehose backend is not *InMemoryBackend; subscription delivery to firehose disabled")
		}
	}

	cwlogsBk.SetSubscriptionDeliverer(d)
}

// cwlogsSubscriptionDeliverer delivers CloudWatch Logs subscription filter payloads to
// Lambda, Kinesis, and Firehose destinations by parsing the destination ARN.
type cwlogsSubscriptionDeliverer struct {
	lambda   *lambdabackend.InMemoryBackend
	kinesis  *kinesisbackend.InMemoryBackend
	firehose *firehosebackend.InMemoryBackend
}

func (d *cwlogsSubscriptionDeliverer) DeliverLogEvents(
	ctx context.Context, destinationArn string, payload []byte,
) error {
	// ARN format: arn:aws:<service>:<region>:<account>:<resource>
	const arnParts = 6
	parts := strings.SplitN(destinationArn, ":", arnParts)
	const arnServiceIdx = 2
	const arnResourceIdx = 5

	if len(parts) < arnParts {
		return nil
	}

	service := parts[arnServiceIdx]
	resource := parts[arnResourceIdx]

	switch service {
	case "lambda":
		if d.lambda == nil {
			return nil
		}
		// resource is "function:<name>" or just "<name>"
		funcName := strings.TrimPrefix(resource, "function:")
		_, _, err := d.lambda.InvokeFunction(
			ctx,
			funcName,
			lambdabackend.InvocationTypeEvent,
			payload,
		)

		return err
	case kinesisServiceName:
		if d.kinesis == nil {
			return nil
		}
		// resource is "stream/<name>"
		streamName := strings.TrimPrefix(resource, "stream/")
		_, err := d.kinesis.PutRecord(ctx, &kinesisbackend.PutRecordInput{
			StreamName:   streamName,
			PartitionKey: "cwlogs",
			Data:         payload,
		})

		return err
	case "firehose":
		if d.firehose == nil {
			return nil
		}
		// resource is "deliverystream/<name>"
		streamName := strings.TrimPrefix(resource, "deliverystream/")

		return d.firehose.PutRecord(ctx, streamName, payload)
	}

	return nil
}

// wireCWLogsMetricEmitter connects the CloudWatch Logs backend to the CloudWatch Metrics backend
// so that metric filter matches on PutLogEvents are forwarded as CloudWatch metric data points.
func wireCWLogsMetricEmitter(cwlogsReg, cwReg service.Registerable) {
	cwlogsH, ok := cwlogsReg.(*cwlogsbackend.Handler)
	if !ok {
		return
	}

	cwlogsBk, bkOk := cwlogsH.Backend.(*cwlogsbackend.InMemoryBackend)
	if !bkOk {
		return
	}

	cwH, cwOk := cwReg.(*cwbackend.Handler)
	if !cwOk {
		return
	}

	cwBk, cwBkOk := cwH.Backend.(*cwbackend.InMemoryBackend)
	if !cwBkOk {
		return
	}

	cwlogsBk.SetMetricEmitter(
		cwlogsbackend.MetricEmitterFunc(
			func(namespace, name string, value float64, unit string) error {
				err := cwBk.PutMetricData(namespace, []cwbackend.MetricDatum{
					{
						MetricName: name,
						Namespace:  namespace,
						Value:      value,
						Unit:       unit,
						Timestamp:  time.Now(),
					},
				})

				return err
			},
		),
	)
}

// wireIAMToSTS connects the IAM backend to STS so that AssumeRole can validate
// ExternalId conditions and enforce per-role MaxSessionDuration limits.
func wireIAMToSTS(iamReg, stsReg service.Registerable) {
	iamH, iamOk := iamReg.(*iambackend.Handler)
	stsH, stsOk := stsReg.(*stsbackend.Handler)

	if !iamOk || !stsOk {
		return
	}

	iamBk, iamBkOk := iamH.Backend.(*iambackend.InMemoryBackend)
	stsBk, stsBkOk := stsH.Backend.(*stsbackend.InMemoryBackend)

	if !iamBkOk || !stsBkOk {
		return
	}

	stsBk.SetRoleLookup(&iamRoleLookupAdapter{backend: iamBk})
	stsBk.SetOIDCLookup(iamBk)
}

// iamRoleLookupAdapter adapts the IAM backend to the STS RoleLookup interface.
type iamRoleLookupAdapter struct {
	backend *iambackend.InMemoryBackend
}

// GetRoleByArn looks up the IAM role by ARN and returns STS-relevant metadata.
func (a *iamRoleLookupAdapter) GetRoleByArn(roleArn string) (*stsbackend.RoleMeta, error) {
	role, err := a.backend.GetRoleByArn(roleArn)
	if err != nil {
		return nil, err
	}

	return &stsbackend.RoleMeta{
		TrustPolicy:        role.AssumeRolePolicyDocument,
		MaxSessionDuration: role.MaxSessionDuration,
	}, nil
}

// GetUserArnByAccessKeyID looks up the IAM user ARN by access key ID.
func (a *iamRoleLookupAdapter) GetUserArnByAccessKeyID(accessKeyID string) (string, error) {
	return a.backend.GetUserArnByAccessKeyID(accessKeyID)
}

// wireSecretsManagerLambda wires the Lambda invoker into the SecretsManager handler
// so that RotateSecret with a RotationLambdaARN invokes the Lambda function.
func wireSecretsManagerLambda(smReg, lambdaReg service.Registerable) {
	smH, ok := smReg.(*secretsmanagerbackend.Handler)
	if !ok {
		return
	}

	if lambdaH, lambdaOk := lambdaReg.(*lambdabackend.Handler); lambdaOk {
		if lambdaBk, bkOk := lambdaH.Backend.(*lambdabackend.InMemoryBackend); bkOk {
			smH.SetLambdaInvoker(lambdaBk)
		}
	}
}

// wireIoTRules connects the IoT rule dispatcher to SQS and Lambda backends, and
// wires the IoT MQTT broker into the IoT Data Plane backend.
func wireIoTRules(iotReg, iotDPReg, sqsReg, lambdaReg service.Registerable) {
	iotH, ok := iotReg.(*iotbackend.Handler)
	if !ok {
		return
	}

	iotBk, bkOk := iotH.Backend.(*iotbackend.InMemoryBackend)
	if !bkOk {
		return
	}

	var sqsBk *sqsbackend.InMemoryBackend
	var lambdaBk *lambdabackend.InMemoryBackend

	if sqsH, sqsOk := sqsReg.(*sqsbackend.Handler); sqsOk {
		sqsBk, _ = sqsH.Backend.(*sqsbackend.InMemoryBackend)
	}

	if lambdaH, lamOk := lambdaReg.(*lambdabackend.Handler); lamOk {
		lambdaBk, _ = lambdaH.Backend.(*lambdabackend.InMemoryBackend)
	}

	iotBk.SetRuleDispatcher(&iotRuleDispatcher{sqs: sqsBk, lambda: lambdaBk})

	// Wire the MQTT broker into the IoT Data Plane backend.
	if iotDPReg != nil {
		if dpH, dpOk := iotDPReg.(*iotdataplanebackend.Handler); dpOk {
			if dpBk, dpBkOk := dpH.Backend.(*iotdataplanebackend.InMemoryBackend); dpBkOk {
				dpBk.SetBroker(iotH.Broker())
			}
		}
	}
}

// wireAppSyncLambda connects the AppSync backend to the Lambda backend so that
// LAMBDA data source resolvers can invoke Lambda functions.
func wireAppSyncLambda(appSyncReg, lambdaReg service.Registerable) {
	appSyncH, ok := appSyncReg.(*appsyncbackend.Handler)
	if !ok {
		return
	}

	appSyncBk, bkOk := appSyncH.Backend.(*appsyncbackend.InMemoryBackend)
	if !bkOk {
		return
	}

	if lambdaH, lambdaOk := lambdaReg.(*lambdabackend.Handler); lambdaOk {
		if lambdaBk, bk2Ok := lambdaH.Backend.(*lambdabackend.InMemoryBackend); bk2Ok {
			appSyncBk.SetLambdaInvoker(lambdaBk)
		}
	}
}

// iotRuleDispatcher adapts the SQS and Lambda backends to the IoT RuleDispatcher interface.
type iotRuleDispatcher struct {
	sqs    *sqsbackend.InMemoryBackend
	lambda *lambdabackend.InMemoryBackend
}

func (d *iotRuleDispatcher) SendToSQS(queueURL, body string) error {
	if d.sqs == nil {
		return nil
	}

	_, err := d.sqs.SendMessage(&sqsbackend.SendMessageInput{
		QueueURL:    queueURL,
		MessageBody: body,
	})

	return err
}

// wireAppSyncDynamoDB connects the AppSync backend to the DynamoDB backend so that
// AMAZON_DYNAMODB data source resolvers can perform GetItem/PutItem operations.
func wireAppSyncDynamoDB(appSyncReg, ddbReg service.Registerable) {
	appSyncH, ok := appSyncReg.(*appsyncbackend.Handler)
	if !ok {
		return
	}

	appSyncBk, bkOk := appSyncH.Backend.(*appsyncbackend.InMemoryBackend)
	if !bkOk {
		return
	}

	if ddbH, ddbOk := ddbReg.(*ddbbackend.DynamoDBHandler); ddbOk {
		if ddbBk, bk3Ok := ddbH.Backend.(*ddbbackend.InMemoryDB); bk3Ok {
			appSyncBk.SetDynamoDBBackend(&dynamoDBAdapter{db: ddbBk})
		}
	}
}

// dynamoDBAdapter adapts ddbbackend.InMemoryDB to the appsync.DynamoDBBackend interface
// by converting between the wire (map[string]any) format and the SDK AttributeValue format.
type dynamoDBAdapter struct {
	db *ddbbackend.InMemoryDB
}

func (a *dynamoDBAdapter) GetItemRaw(
	ctx context.Context,
	tableName string,
	key map[string]any,
) (map[string]any, error) {
	sdkKey, err := ddbmodels.ToSDKItem(key)
	if err != nil {
		return nil, fmt.Errorf("appsync ddb adapter: marshal key: %w", err)
	}

	out, err := a.db.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &tableName,
		Key:       sdkKey,
	})
	if err != nil {
		return nil, err
	}

	if len(out.Item) == 0 {
		return map[string]any{}, nil
	}

	return ddbmodels.FromSDKItem(out.Item), nil
}

func (a *dynamoDBAdapter) PutItemRaw(
	ctx context.Context,
	tableName string,
	item map[string]any,
) error {
	sdkItem, err := ddbmodels.ToSDKItem(item)
	if err != nil {
		return fmt.Errorf("appsync ddb adapter: marshal item: %w", err)
	}

	_, err = a.db.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &tableName,
		Item:      sdkItem,
	})

	return err
}

func (d *iotRuleDispatcher) InvokeLambda(
	ctx context.Context,
	functionARN string,
	payload []byte,
) error {
	if d.lambda == nil {
		return nil
	}

	_, _, err := d.lambda.InvokeFunction(
		ctx,
		functionARN,
		lambdabackend.InvocationTypeEvent,
		payload,
	)

	return err
}

// arnServiceIs returns true if the ARN's service segment (the third colon-delimited field)
// matches the given service name exactly. This is more precise than a substring search since
// ARN format is "arn:aws:SERVICE:REGION:ACCOUNT:RESOURCE".
func arnServiceIs(a, serviceName string) bool {
	// Fast path: ARN must start with "arn:aws:" (or "arn:aws-cn:", "arn:aws-us-gov:", etc.)
	// We split on ":" up to 3 parts to extract just the service field.
	start := strings.Index(a, ":")
	if start < 0 {
		return false
	}

	start++ // skip past first ":"

	next := strings.Index(a[start:], ":")
	if next < 0 {
		return false
	}

	start += next + 1 // skip past second ":"

	end := strings.Index(a[start:], ":")
	if end < 0 {
		return false
	}

	return a[start:start+end] == serviceName
}

// registerTaggingService wires a single service's provider, ARN tagger, and ARN untagger into
// the Resource Groups Tagging API backend. arnService is the AWS service name used to match
// the service segment of an ARN (e.g., "sqs", "sns", "lambda").
func registerTaggingService(
	bk resourcegroupstaggingapibackend.StorageBackend,
	provider resourcegroupstaggingapibackend.ResourceProvider,
	arnService string,
	tagger func(context.Context, string, map[string]string) error,
	untagger func(context.Context, string, []string) error,
) {
	bk.RegisterProvider(provider)
	bk.RegisterARNTagger(func(ctx context.Context, arn string, newTags map[string]string) (bool, error) {
		if !arnServiceIs(arn, arnService) {
			return false, nil
		}

		return true, tagger(ctx, arn, newTags)
	})
	bk.RegisterARNUntagger(func(ctx context.Context, arn string, keys []string) (bool, error) {
		if !arnServiceIs(arn, arnService) {
			return false, nil
		}

		return true, untagger(ctx, arn, keys)
	})
}

// wireResourceGroupsTagging connects the Resource Groups Tagging API backend to all
// service backends so that GetResources, GetTagKeys, GetTagValues, TagResources, and
// UntagResources work cross-service.
//
// Coverage note (bd: gopherstack-3xne, gopherstack-7rsk, gopherstack-no6n, gopherstack-91e0,
// gopherstack-8kco, gopherstack-pdqm): of the ~90 gopherstack services with native tagging
// support, this wires 99 (dynamodb, sqs, sns, lambda, kms, secretsmanager, ecs, athena, glue,
// ecr, kinesis, stepfunctions, cloudfront, eks, batch, wafv2, backup, efs, docdb, neptune, rds,
// elasticache, redshift, sagemaker, firehose, opensearch, cloudwatchlogs, mq, emr, grafana,
// outposts, resiliencehub, directconnect, mgn, networkmanager, lightsail, dax,
// detective, guardduty, transfer, cognitoidp, appconfig, codecommit,
// servicediscovery, memorydb, accessanalyzer, dlm, ce, mediapackage, swf, fis,
// codeconnections, mediastore, mwaa, pipes, macie2, managedblockchain, mediaconvert,
// datasync, codedeploy, inspector2, ram, rekognition, translate, appstream,
// mediatailor, vpclattice, codepipeline, kinesisanalyticsv2, opsworks, comprehend,
// shield, transcribe, verifiedpermissions, waf, securityhub, apprunner,
// route53resolver, timestreamwrite, s3tables, s3, s3control, workmail, pinpoint,
// applicationautoscaling, codeartifact, cleanrooms, appmesh, personalize, sesv2,
// xray, awsconfig, scheduler, appsync, emrserverless, acm, ssoadmin, apigateway,
// organizations). s3control is wired
// only for the resource kinds taggable through its generic TagResource/UntagResource/
// ListTagsForResource ops (access points, Object Lambda access points, multi-region access
// points, access grants) -- see wireTaggingS3Control's doc comment for why batch job tags,
// Storage Lens configuration tags, and Outposts bucket tags (each a separate real store
// behind its own dedicated AWS op) are out of scope. emrserverless is wired only for
// applications and job runs -- see wireTaggingEmrServerless's doc comment for why sessions
// are out of scope. acm additionally covers acme-endpoint, acme-external-account-binding,
// and acme-domain-validation resources alongside certificates -- see wireTaggingACM's doc
// comment for why acme-account is excluded. organizations is wired only for account, root,
// OU, and policy -- see wireTaggingOrganizations's doc comment for why the organization
// resource itself is excluded. The rest remain unwired -- see PARITY.md's gaps
// section for the honest remaining list and why a few (notably codebuild, whose real API has
// no TagResource/CreateTags-style mutation call at
// all -- tags are set only via CreateProject/UpdateProject/CreateFleet/UpdateFleet/
// CreateReportGroup request bodies; and forecast, whose only resource-creation path is an
// unexported backend method reachable solely through its own JSON operation dispatch) need
// more than this dispatch shape supports today.
//
// byName supplies every dependency by service.Registerable.Name() (e.g. byName["DynamoDB"])
// rather than one parameter per service: the wired-service count keeps growing, and a
// positional parameter list that size (20+ and climbing) is worse to read and extend than
// one map lookup per wireTaggingXxx call below.
func wireResourceGroupsTagging(taggingReg service.Registerable, byName map[string]service.Registerable) {
	taggingH, ok := taggingReg.(*resourcegroupstaggingapibackend.Handler)
	if !ok {
		return
	}

	bk := taggingH.Backend

	wireResourceGroupsTaggingCore(bk, byName)
	wireResourceGroupsTaggingData(bk, byName)
	wireResourceGroupsTaggingInfra(bk, byName)
	wireResourceGroupsTaggingMisc(bk, byName)
	wireResourceGroupsTaggingApps(bk, byName)
	wireResourceGroupsTaggingExtra(bk, byName)
	wireResourceGroupsTaggingSweep5(bk, byName)
	wireResourceGroupsTaggingSweep6(bk, byName)
	wireResourceGroupsTaggingPolicy(bk, byName["Organizations"])
}

// wireResourceGroupsTaggingPolicy registers Organizations' effective TAG_POLICY
// document as the source ListRequiredTags derives required-tag rows from (see
// services/resourcegroupstaggingapi/cross_service.go's TagPolicyProvider and
// compliance.go's requiredTagsFromPolicy). Central wiring, not the tagging
// service's own concern: it is the only place that knows about both backends.
func wireResourceGroupsTaggingPolicy(bk resourcegroupstaggingapibackend.StorageBackend, orgReg service.Registerable) {
	orgH, ok := orgReg.(*organizationsbackend.Handler)
	if !ok || orgH.Backend == nil {
		return
	}

	bk.RegisterTagPolicyProvider(func() (string, bool) {
		ep, err := orgH.Backend.DescribeEffectivePolicy("TAG_POLICY", "")
		if err != nil {
			return "", false
		}

		return ep.PolicyContent, true
	})
}

// wireResourceGroupsTaggingCore wires the original core set of tagging services
// (dynamodb, sqs, sns, lambda, kms, secretsmanager) plus the first sweep's
// compute/orchestration/CDN services, split out of wireResourceGroupsTagging to keep
// it under this repo's funlen limit.
func wireResourceGroupsTaggingCore(
	bk resourcegroupstaggingapibackend.StorageBackend,
	byName map[string]service.Registerable,
) {
	wireTaggingDDB(bk, byName["DynamoDB"])
	wireTaggingSQS(bk, byName["SQS"])
	wireTaggingSNS(bk, byName["SNS"])
	wireTaggingLambda(bk, byName["Lambda"])
	wireTaggingKMS(bk, byName["KMS"])
	wireTaggingSM(bk, byName["SecretsManager"])
	wireTaggingECS(bk, byName["ECS"])
	wireTaggingAthena(bk, byName["Athena"])
	wireTaggingGlue(bk, byName["Glue"])
	wireTaggingECR(bk, byName["ECR"])
	wireTaggingKinesis(bk, byName["Kinesis"])
	wireTaggingStepFunctions(bk, byName["StepFunctions"])
	wireTaggingCloudFront(bk, byName["CloudFront"])
	wireTaggingEKS(bk, byName["EKS"])
	wireTaggingBatch(bk, byName["Batch"])
	wireTaggingWAFv2(bk, byName["Wafv2"])
	wireTaggingBackup(bk, byName["Backup"])
	wireTaggingEFS(bk, byName["EFS"])
}

// wireResourceGroupsTaggingData wires the data-store services (DocDB, Neptune, RDS,
// ElastiCache), split out of wireResourceGroupsTagging to keep it under this repo's
// funlen limit.
func wireResourceGroupsTaggingData(
	bk resourcegroupstaggingapibackend.StorageBackend,
	byName map[string]service.Registerable,
) {
	// DocDB and Neptune must be wired ahead of RDS: both share the "rds" ARN service
	// for some or all of their resource kinds (see wireTaggingDocDB and
	// wireTaggingNeptune below), and resourcegroupstaggingapi tries ARN taggers in
	// registration order, stopping at the first handled=true match. RDS's own tagger
	// does not validate resource existence and would otherwise either shadow these
	// (if registered first) or get shadowed uselessly by them (if these ran
	// unconditionally after it).
	wireTaggingDocDB(bk, byName["DocDB"])
	wireTaggingNeptune(bk, byName["Neptune"])
	wireTaggingRDS(bk, byName["RDS"])
	wireTaggingElastiCache(bk, byName["ElastiCache"])
}

// wireResourceGroupsTaggingInfra wires the gopherstack-no6n re-audit sweep's services
// plus DAX/Detective/GuardDuty/Transfer/CognitoIDP/AppConfig/CodeCommit/
// ServiceDiscovery/MemoryDB, split out of wireResourceGroupsTagging to keep it under
// this repo's funlen limit.
func wireResourceGroupsTaggingInfra(
	bk resourcegroupstaggingapibackend.StorageBackend,
	byName map[string]service.Registerable,
) {
	// gopherstack-no6n: these eight were reported as untaggable by a grep for
	// "func.*TagResource(" that missed every non-standard method name (AddTags,
	// CreateTags, TagDeliveryStream, ...). Re-audited: seven have real native tagging
	// and are wired below; codebuild is not (see wireResourceGroupsTagging's package
	// doc, or PARITY.md, for why).
	wireTaggingRedshift(bk, byName["Redshift"])
	wireTaggingSageMaker(bk, byName["SageMaker"])
	wireTaggingFirehose(bk, byName["Firehose"])
	wireTaggingOpenSearch(bk, byName["OpenSearch"])
	wireTaggingCloudWatchLogs(bk, byName["CloudWatchLogs"])
	wireTaggingMQ(bk, byName["MQ"])
	wireTaggingEMR(bk, byName["EMR"])
	wireTaggingGrafana(bk, byName["Grafana"])
	wireTaggingOutposts(bk, byName["Outposts"])
	wireTaggingResilienceHub(bk, byName["ResilienceHub"])
	wireTaggingDirectConnect(bk, byName["DirectConnect"])
	wireTaggingMGN(bk, byName["MGN"])
	wireTaggingNetworkManager(bk, byName["NetworkManager"])
	wireTaggingLightsail(bk, byName["Lightsail"])
}

// wireResourceGroupsTaggingMisc wires DAX, Detective, GuardDuty, Transfer,
// CognitoIDP, AppConfig, CodeCommit, ServiceDiscovery, and MemoryDB, split out of
// wireResourceGroupsTagging to keep it under this repo's funlen limit.
func wireResourceGroupsTaggingMisc(
	bk resourcegroupstaggingapibackend.StorageBackend,
	byName map[string]service.Registerable,
) {
	wireTaggingDAX(bk, byName["DAX"])
	wireTaggingDetective(bk, byName["Detective"])
	wireTaggingGuardDuty(bk, byName["GuardDuty"])
	wireTaggingTransfer(bk, byName["Transfer"])
	wireTaggingCognitoIDP(bk, byName["CognitoIDP"])
	wireTaggingAppConfig(bk, byName["AppConfig"])
	wireTaggingCodeCommit(bk, byName["CodeCommit"])
	wireTaggingServiceDiscovery(bk, byName["ServiceDiscovery"])
	wireTaggingMemoryDB(bk, byName["MemoryDB"])
}

// wireResourceGroupsTaggingApps wires this sweep's services (gopherstack-3xne):
// AccessAnalyzer, DLM, Cost Explorer, MediaPackage, SWF, FIS, CodeConnections,
// MediaStore, MWAA, Pipes, Macie2, ManagedBlockchain, MediaConvert, DataSync,
// CodeDeploy, Inspector2.
func wireResourceGroupsTaggingApps(
	bk resourcegroupstaggingapibackend.StorageBackend,
	byName map[string]service.Registerable,
) {
	wireTaggingAccessAnalyzer(bk, byName["AccessAnalyzer"])
	wireTaggingDLM(bk, byName["DLM"])
	wireTaggingCE(bk, byName["Ce"])
	wireTaggingMediaPackage(bk, byName["MediaPackage"])
	wireTaggingSWF(bk, byName["SWF"])
	wireTaggingFIS(bk, byName["FIS"])
	wireTaggingCodeConnections(bk, byName["CodeConnections"])
	wireTaggingMediaStore(bk, byName["MediaStore"])
	wireTaggingMWAA(bk, byName["MWAA"])
	wireTaggingPipes(bk, byName["Pipes"])
	wireTaggingMacie2(bk, byName["Macie2"])
	wireTaggingManagedBlockchain(bk, byName["ManagedBlockchain"])
	wireTaggingMediaConvert(bk, byName["MediaConvert"])
	wireTaggingDataSync(bk, byName["DataSync"])
	wireTaggingCodeDeploy(bk, byName["CodeDeploy"])
	wireTaggingInspector2(bk, byName["Inspector2"])
}

// wireResourceGroupsTaggingExtra wires this sweep's services (gopherstack-3xne,
// fourth pass): RAM, Rekognition, Translate, AppStream, MediaTailor,
// VPCLattice, CodePipeline, KinesisAnalyticsV2, plus OpsWorks (gopherstack-91e0,
// once it was registered as a running service). Split out (rather than folded
// into wireResourceGroupsTaggingApps) to keep every group under this repo's
// funlen limit.
func wireResourceGroupsTaggingExtra(
	bk resourcegroupstaggingapibackend.StorageBackend,
	byName map[string]service.Registerable,
) {
	wireTaggingRAM(bk, byName["RAM"])
	wireTaggingRekognition(bk, byName["Rekognition"])
	wireTaggingTranslate(bk, byName["Translate"])
	wireTaggingAppStream(bk, byName["AppStream"])
	wireTaggingMediaTailor(bk, byName["MediaTailor"])
	wireTaggingVPCLattice(bk, byName["VPCLattice"])
	wireTaggingCodePipeline(bk, byName["CodePipeline"])
	wireTaggingKinesisAnalyticsV2(bk, byName["KinesisAnalyticsV2"])
	wireTaggingOpsWorks(bk, byName["OpsWorks"])
}

// wireResourceGroupsTaggingSweep5 wires this sweep's services (gopherstack-3xne,
// fifth pass): Comprehend, Shield, Transcribe, VerifiedPermissions, WAF Classic,
// SecurityHub, AppRunner, Route53Resolver, Timestream Write, S3 Tables, WorkMail,
// Pinpoint, Application Auto Scaling, CodeArtifact, Clean Rooms, App Mesh, Personalize,
// SESv2, X-Ray, AWS Config, and EventBridge Scheduler. Split out (rather than folded
// into an existing group) to keep every group under this repo's funlen limit.
func wireResourceGroupsTaggingSweep5(
	bk resourcegroupstaggingapibackend.StorageBackend,
	byName map[string]service.Registerable,
) {
	wireTaggingComprehend(bk, byName["Comprehend"])
	wireTaggingShield(bk, byName["Shield"])
	wireTaggingTranscribe(bk, byName["Transcribe"])
	wireTaggingVerifiedPermissions(bk, byName["VerifiedPermissions"])
	wireTaggingWAF(bk, byName["WAF"])
	wireTaggingSecurityHub(bk, byName["SecurityHub"])
	wireTaggingAppRunner(bk, byName["AppRunner"])
	wireTaggingRoute53Resolver(bk, byName["Route53Resolver"])
	wireTaggingTimestreamWrite(bk, byName["TimestreamWrite"])
	wireTaggingS3Tables(bk, byName["S3tables"])
	wireTaggingS3(bk, byName["S3"])
	wireTaggingS3Control(bk, byName["S3Control"])
	wireTaggingWorkMail(bk, byName["WorkMail"])
	wireTaggingPinpoint(bk, byName["Pinpoint"])
	wireTaggingApplicationAutoScaling(bk, byName["ApplicationAutoscaling"])
	wireTaggingCodeArtifact(bk, byName["CodeArtifact"])
	wireTaggingCleanRooms(bk, byName["CleanRooms"])
	wireTaggingAppMesh(bk, byName["AppMesh"])
	wireTaggingPersonalize(bk, byName["Personalize"])
	wireTaggingSESv2(bk, byName["SESv2"])
	wireTaggingXRay(bk, byName["Xray"])
	wireTaggingAWSConfig(bk, byName["AWSConfig"])
	wireTaggingScheduler(bk, byName["Scheduler"])
}

// wireResourceGroupsTaggingSweep6 wires this sweep's services (gopherstack-pdqm):
// AppSync, EMR Serverless, ACM, SSO Admin, API Gateway, and Organizations. Split
// out (rather than folded into wireResourceGroupsTaggingSweep5) to keep every
// group under this repo's funlen limit.
func wireResourceGroupsTaggingSweep6(
	bk resourcegroupstaggingapibackend.StorageBackend,
	byName map[string]service.Registerable,
) {
	wireTaggingAppSync(bk, byName["AppSync"])
	wireTaggingEmrServerless(bk, byName["EmrServerless"])
	wireTaggingACM(bk, byName["ACM"])
	wireTaggingSSOAdmin(bk, byName["SsoAdmin"])
	wireTaggingAPIGateway(bk, byName["APIGateway"])
	wireTaggingOrganizations(bk, byName["Organizations"])
}

func wireTaggingDDB(
	bk resourcegroupstaggingapibackend.StorageBackend,
	ddbReg service.Registerable,
) {
	ddbH, ok := ddbReg.(*ddbbackend.DynamoDBHandler)
	if !ok {
		return
	}

	ddbBk, ok := ddbH.Backend.(*ddbbackend.InMemoryDB)
	if !ok {
		return
	}

	registerTaggingService(
		bk,
		func(_ context.Context) []resourcegroupstaggingapibackend.TaggedResource {
			tables := ddbBk.TaggedTables()
			out := make([]resourcegroupstaggingapibackend.TaggedResource, 0, len(tables))
			for _, t := range tables {
				out = append(out, resourcegroupstaggingapibackend.TaggedResource{
					ResourceARN:  t.ARN,
					ResourceType: "dynamodb:table",
					Tags:         t.Tags,
				})
			}

			return out
		},
		"dynamodb",
		func(ctx context.Context, arn string, newTags map[string]string) error {
			sdkTags := make([]ddbsdktypes.Tag, 0, len(newTags))
			for k, v := range newTags {
				tagKey, tagValue := k, v
				sdkTags = append(sdkTags, ddbsdktypes.Tag{Key: &tagKey, Value: &tagValue})
			}

			_, err := ddbBk.TagResource(ctx, &dynamodb.TagResourceInput{
				ResourceArn: aws.String(arn),
				Tags:        sdkTags,
			})

			return err
		},
		func(ctx context.Context, arn string, keys []string) error {
			_, err := ddbBk.UntagResource(ctx, &dynamodb.UntagResourceInput{
				ResourceArn: aws.String(arn),
				TagKeys:     keys,
			})

			return err
		},
	)
}

// taggedARNEntry holds an ARN and its tag map for cross-service tagging helpers.
type taggedARNEntry struct {
	Tags map[string]string
	ARN  string
}

// mapToTagSlice converts a tag map into a slice of a service's own Key/Value tag
// struct type via newTag, for services whose native TagResource takes []T rather than
// map[string]string. Shared by every wireTaggingXxx that needs this conversion so the
// conversion loop isn't duplicated per service (each service's tag struct is a
// distinct named type, so the loop itself can't be shared without this).
func mapToTagSlice[T any](tags map[string]string, newTag func(key, value string) T) []T {
	out := make([]T, 0, len(tags))
	for k, v := range tags {
		out = append(out, newTag(k, v))
	}

	return out
}

// wireTaggingARNResources registers a tagging service whose resources are described by
// a slice of taggedARNEntry values. arnService is passed to arnServiceIs (e.g. "sqs").
// resourceTypeOf computes the AWS resource-type string (e.g. "sqs:queue") for a given
// ARN; services whose flat ARN-keyed tag store holds exactly one resource kind can pass
// a closure that ignores its argument and returns a constant, while services that mix
// several kinds under one store (e.g. ECS clusters/services/task-definitions) can pass
// [resourceTypeFromARN] to derive the type per-ARN instead of hand-writing one wiring
// function per kind.
func wireTaggingARNResources(
	bk resourcegroupstaggingapibackend.StorageBackend,
	arnService string,
	resourceTypeOf func(arn string) string,
	listFn func() []taggedARNEntry,
	tagFn func(string, map[string]string) error,
	untagFn func(string, []string) error,
) {
	registerTaggingService(
		bk,
		func(_ context.Context) []resourcegroupstaggingapibackend.TaggedResource {
			items := listFn()
			out := make([]resourcegroupstaggingapibackend.TaggedResource, 0, len(items))
			for _, item := range items {
				out = append(out, resourcegroupstaggingapibackend.TaggedResource{
					ResourceARN:  item.ARN,
					ResourceType: resourceTypeOf(item.ARN),
					Tags:         item.Tags,
				})
			}

			return out
		},
		arnService,
		func(_ context.Context, arn string, newTags map[string]string) error {
			return tagFn(arn, newTags)
		},
		func(_ context.Context, arn string, keys []string) error {
			return untagFn(arn, keys)
		},
	)
}

// wireTaggingCtxARNResources is wireTaggingARNResources for services whose native
// TagResource/UntagResource take a context.Context as their first parameter (used
// elsewhere by those services for region or endpoint resolution) instead of the bare
// (arn, tags)/(arn, keys) shape wireTaggingARNResources expects. Kept as a separate
// helper (rather than a variant of wireTaggingARNResources) so ctx-less callers keep a
// ctx-less tagFn/untagFn signature.
func wireTaggingCtxARNResources(
	bk resourcegroupstaggingapibackend.StorageBackend,
	arnService string,
	resourceTypeOf func(arn string) string,
	listFn func() []taggedARNEntry,
	tagFn func(context.Context, string, map[string]string) error,
	untagFn func(context.Context, string, []string) error,
) {
	registerTaggingService(
		bk,
		func(_ context.Context) []resourcegroupstaggingapibackend.TaggedResource {
			items := listFn()
			out := make([]resourcegroupstaggingapibackend.TaggedResource, 0, len(items))
			for _, item := range items {
				out = append(out, resourcegroupstaggingapibackend.TaggedResource{
					ResourceARN:  item.ARN,
					ResourceType: resourceTypeOf(item.ARN),
					Tags:         item.Tags,
				})
			}

			return out
		},
		arnService,
		tagFn,
		untagFn,
	)
}

// constantResourceType returns a resourceTypeOf closure (see wireTaggingARNResources)
// that ignores the ARN and always returns t, for services whose flat ARN-keyed tag
// store holds exactly one AWS resource kind.
func constantResourceType(t string) func(string) string {
	return func(string) string { return t }
}

// arnResourceFieldCount is the number of colon-delimited fields in a well-formed AWS
// ARN: "arn:partition:service:region:account-id:resource".
const arnResourceFieldCount = 6

// resourceTypeFromARN derives an AWS resource-type string (e.g. "ecs:cluster") from
// arn's own resource segment, for services whose flat ARN-keyed tag store spans more
// than one resource kind. By AWS's own ARN convention the resource segment is either
// "type/id" (e.g. "cluster/my-cluster") or "type:id" (e.g. "function:my-fn"); this
// takes the portion before the first "/" or ":" and prefixes it with service. Falls
// back to service alone when the ARN is malformed or its resource segment has no
// type/id separator (matching the constant-type case for a single-kind resource,
// e.g. SQS/SNS whose resource segment is the bare resource name).
func resourceTypeFromARN(arn, service string) string {
	parts := strings.SplitN(arn, ":", arnResourceFieldCount)
	if len(parts) != arnResourceFieldCount {
		return service
	}

	resource := parts[arnResourceFieldCount-1]

	if idx := strings.IndexAny(resource, "/:"); idx >= 0 {
		return service + ":" + resource[:idx]
	}

	return service
}

// wafv2ResourceType derives the resource-type string for a WAFv2 ARN. Unlike the other
// services this file wires, WAFv2's resource segment nests the scope one level deeper
// than resourceTypeFromARN accounts for -- "{regional|global}/{kind}/{name}/{id}" (see
// InMemoryBackend.buildWebACLARN/ipSetARN/regexPatternSetARN/ruleGroupARN) -- so taking
// only the first "/"-delimited segment would yield "wafv2:regional" for every kind,
// silently colliding web ACLs, IP sets, regex pattern sets, and rule groups under one
// type and never matching AWS's real "wafv2:regional/webacl" style resource-type
// strings. This takes the first two segments instead.
func wafv2ResourceType(resourceARN string) string {
	parts := strings.SplitN(resourceARN, ":", arnResourceFieldCount)
	if len(parts) != arnResourceFieldCount {
		return "wafv2"
	}

	segs := strings.SplitN(parts[arnResourceFieldCount-1], "/", wafv2ResourceSegmentCount+1)
	if len(segs) < wafv2ResourceSegmentCount {
		return "wafv2"
	}

	return "wafv2:" + segs[0] + "/" + segs[1]
}

// wafv2ResourceSegmentCount is the number of "/"-delimited segments
// (scope, kind) wafv2ResourceType needs from a WAFv2 ARN's resource
// portion before the name/id segments it discards.
const wafv2ResourceSegmentCount = 2

// nestedResourceARNSegmentCount is the "/"-delimited segment count of a
// "parent/id/kind/id" nested ARN (see nestedResourceType).
const nestedResourceARNSegmentCount = 4

// nestedResourceType is resourceTypeFromARN for services whose flat ARN-keyed tag
// store mixes a top-level resource kind with a second kind nested one level beneath
// it ("parent/id/kind/id", e.g. GuardDuty's "detector/id/filter/id" or AppConfig's
// "application/id/environment/id"): a plain resourceTypeFromARN would take only the
// first segment and silently collide every nested kind under the parent's type,
// the same class of bug wafv2ResourceType exists to avoid. Returns the third segment
// for a 4-segment resource ("kind" in "parent/id/kind/id"), otherwise falls back to
// resourceTypeFromARN's first-segment rule (also correct for a bare, non-nested
// resource name with no "/" at all, e.g. CodePipeline's pipeline ARNs).
func nestedResourceType(resourceARN, service string) string {
	parts := strings.SplitN(resourceARN, ":", arnResourceFieldCount)
	if len(parts) != arnResourceFieldCount {
		return service
	}

	segs := strings.Split(parts[arnResourceFieldCount-1], "/")
	if len(segs) == nestedResourceARNSegmentCount {
		return service + ":" + segs[2]
	}

	return resourceTypeFromARN(resourceARN, service)
}

// s3tablesResourceType derives the resource-type string for an S3 Tables ARN.
// Buckets are flat ("bucket/{name}", 2 segments) but tables nest one level
// deeper under a namespace ("bucket/{bucket}/table/{namespace}/{table}", 5
// segments -- see InMemoryBackend.TableARN) -- neither resourceTypeFromARN
// (would read every table as a bucket) nor nestedResourceType (only handles
// exactly 4 segments) fits, so this checks for the literal "table" segment.
func s3tablesResourceType(resourceARN string) string {
	parts := strings.SplitN(resourceARN, ":", arnResourceFieldCount)
	if len(parts) != arnResourceFieldCount {
		return "s3tables"
	}

	segs := strings.Split(parts[arnResourceFieldCount-1], "/")
	if len(segs) >= s3tablesTableSegmentCount && segs[2] == "table" {
		return "s3tables:table"
	}

	return resourceTypeFromARN(resourceARN, "s3tables")
}

// s3tablesTableSegmentCount is the minimum "/"-delimited segment count of a
// table ARN ("bucket", "{bucket}", "table", ...) that s3tablesResourceType
// checks before reading the literal "table" segment.
const s3tablesTableSegmentCount = 3

// appmeshResourceType derives the resource-type string for an App Mesh ARN.
// Mesh sub-resources nest to varying depths -- a mesh itself is 2 segments
// ("mesh/{name}"), most sub-resources (virtualNode, virtualRouter,
// virtualGateway, virtualService) are 4 ("mesh/{name}/virtualNode/{name}"),
// and routes/gateway routes nest one level deeper still, at 6
// ("mesh/{name}/virtualRouter/{name}/route/{name}", see routeARN/
// gatewayRouteARN in virtual_routers.go/virtual_gateways.go) -- deeper than
// nestedResourceType's fixed 4-segment check handles. For any even segment
// count the innermost resource kind is always the second-to-last segment,
// so this generalizes that rule instead of hardcoding a depth.
func appmeshResourceType(resourceARN string) string {
	parts := strings.SplitN(resourceARN, ":", arnResourceFieldCount)
	if len(parts) != arnResourceFieldCount {
		return "appmesh"
	}

	segs := strings.Split(parts[arnResourceFieldCount-1], "/")
	if len(segs) >= appmeshMinNestedSegmentCount && len(segs)%appmeshSegmentPairSize == 0 {
		return "appmesh:" + segs[len(segs)-2]
	}

	return "appmesh"
}

// appmeshMinNestedSegmentCount and appmeshSegmentPairSize bound
// appmeshResourceType's "even segment count" check: a well-formed App Mesh
// resource segment is always a sequence of "kind/id" pairs.
const (
	appmeshMinNestedSegmentCount = 2
	appmeshSegmentPairSize       = 2
)

func wireTaggingSQS(
	bk resourcegroupstaggingapibackend.StorageBackend,
	sqsReg service.Registerable,
) {
	sqsH, ok := sqsReg.(*sqsbackend.Handler)
	if !ok {
		return
	}

	sqsBk, ok := sqsH.Backend.(*sqsbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "sqs", constantResourceType("sqs:queue"),
		func() []taggedARNEntry {
			queues := sqsBk.TaggedQueues()
			out := make([]taggedARNEntry, 0, len(queues))
			for _, q := range queues {
				out = append(out, taggedARNEntry{ARN: q.ARN, Tags: q.Tags})
			}

			return out
		},
		sqsBk.TagQueueByARN,
		sqsBk.UntagQueueByARN,
	)
}

func wireTaggingSNS(
	bk resourcegroupstaggingapibackend.StorageBackend,
	snsReg service.Registerable,
) {
	snsH, ok := snsReg.(*snsbackend.Handler)
	if !ok {
		return
	}

	snsBk, ok := snsH.Backend.(*snsbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "sns", constantResourceType("sns:topic"),
		func() []taggedARNEntry {
			topics := snsBk.TaggedTopics()
			out := make([]taggedARNEntry, 0, len(topics))
			for _, t := range topics {
				out = append(out, taggedARNEntry{ARN: t.ARN, Tags: t.Tags})
			}

			return out
		},
		snsBk.TagTopicByARN,
		snsBk.UntagTopicByARN,
	)
}

func wireTaggingLambda(
	bk resourcegroupstaggingapibackend.StorageBackend,
	lambdaReg service.Registerable,
) {
	lambdaH, ok := lambdaReg.(*lambdabackend.Handler)
	if !ok {
		return
	}

	registerTaggingService(
		bk,
		func(ctx context.Context) []resourcegroupstaggingapibackend.TaggedResource {
			fns := lambdaH.TaggedFunctions(ctx)
			out := make([]resourcegroupstaggingapibackend.TaggedResource, 0, len(fns))
			for _, f := range fns {
				out = append(out, resourcegroupstaggingapibackend.TaggedResource{
					ResourceARN:  f.ARN,
					ResourceType: "lambda:function",
					Tags:         f.Tags,
				})
			}

			return out
		},
		"lambda",
		lambdaH.TagFunctionByARN,
		lambdaH.UntagFunctionByARN,
	)
}

func wireTaggingKMS(
	bk resourcegroupstaggingapibackend.StorageBackend,
	kmsReg service.Registerable,
) {
	kmsH, ok := kmsReg.(*kmsbackend.Handler)
	if !ok {
		return
	}

	registerTaggingService(
		bk,
		func(ctx context.Context) []resourcegroupstaggingapibackend.TaggedResource {
			keys := kmsH.TaggedKeys(ctx)
			out := make([]resourcegroupstaggingapibackend.TaggedResource, 0, len(keys))
			for _, k := range keys {
				out = append(out, resourcegroupstaggingapibackend.TaggedResource{
					ResourceARN:  k.ARN,
					ResourceType: "kms:key",
					Tags:         k.Tags,
				})
			}

			return out
		},
		"kms",
		kmsH.TagKeyByARN,
		kmsH.UntagKeyByARN,
	)
}

func wireTaggingSM(bk resourcegroupstaggingapibackend.StorageBackend, smReg service.Registerable) {
	smH, ok := smReg.(*secretsmanagerbackend.Handler)
	if !ok {
		return
	}

	smBk, ok := smH.Backend.(*secretsmanagerbackend.InMemoryBackend)
	if !ok {
		return
	}

	registerTaggingService(
		bk,
		func(ctx context.Context) []resourcegroupstaggingapibackend.TaggedResource {
			secrets := smBk.TaggedSecrets(ctx)
			out := make([]resourcegroupstaggingapibackend.TaggedResource, 0, len(secrets))
			for _, s := range secrets {
				out = append(out, resourcegroupstaggingapibackend.TaggedResource{
					ResourceARN:  s.ARN,
					ResourceType: "secretsmanager:secret",
					Tags:         s.Tags,
				})
			}

			return out
		},
		"secretsmanager",
		smBk.TagSecretByARN,
		smBk.UntagSecretByARN,
	)
}

// wireTaggingECS wires the ECS backend into the Resource Groups Tagging API. ECS keeps
// tags for every resource kind it supports (clusters, services, task definitions,
// container instances, task sets, capacity providers) in one flat ARN-keyed side map,
// so resourceTypeFromARN derives the per-resource type instead of a hand-written
// wiring function per ECS resource kind.
func wireTaggingECS(bk resourcegroupstaggingapibackend.StorageBackend, ecsReg service.Registerable) {
	ecsH, ok := ecsReg.(*ecsbackend.Handler)
	if !ok {
		return
	}

	ecsBk, ok := ecsH.Backend.(*ecsbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "ecs",
		func(arn string) string { return resourceTypeFromARN(arn, "ecs") },
		func() []taggedARNEntry {
			items := ecsBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		func(arn string, newTags map[string]string) error {
			tagList := make([]ecsbackend.Tag, 0, len(newTags))
			for k, v := range newTags {
				tagList = append(tagList, ecsbackend.Tag{Key: k, Value: v})
			}

			return ecsBk.TagResource(arn, tagList)
		},
		ecsBk.UntagResource,
	)
}

// wireTaggingAthena wires the Athena backend into the Resource Groups Tagging API.
// Like ECS, Athena keeps tags for every resource kind it supports (workgroups, data
// catalogs, capacity reservations, notebooks) in one flat ARN-keyed map, so
// resourceTypeFromARN derives the per-resource type.
func wireTaggingAthena(bk resourcegroupstaggingapibackend.StorageBackend, athenaReg service.Registerable) {
	athenaH, ok := athenaReg.(*athenabackend.Handler)
	if !ok {
		return
	}

	athenaBk, ok := athenaH.Backend.(*athenabackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "athena",
		func(arn string) string { return resourceTypeFromARN(arn, "athena") },
		func() []taggedARNEntry {
			items := athenaBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		athenaBk.TagResource,
		athenaBk.UntagResource,
	)
}

// wireTaggingGlue wires the Glue backend into the Resource Groups Tagging API. Glue
// has the widest resource-kind spread of the services wired this pass (databases,
// crawlers, jobs, data quality rulesets, connections, triggers, workflows), all under
// the same "glue:type/id" ARN convention, so resourceTypeFromARN derives the type for
// every kind uniformly instead of one wiring function apiece.
func wireTaggingGlue(bk resourcegroupstaggingapibackend.StorageBackend, glueReg service.Registerable) {
	glueH, ok := glueReg.(*gluebackend.Handler)
	if !ok {
		return
	}

	glueBk, ok := glueH.Backend.(*gluebackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "glue",
		func(arn string) string { return resourceTypeFromARN(arn, "glue") },
		func() []taggedARNEntry {
			items := glueBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		glueBk.TagResource,
		glueBk.UntagResource,
	)
}

// wireTaggingECR wires the ECR backend into the Resource Groups Tagging API. ECR only
// exposes repository ARNs for tagging (a single resource kind), matching real AWS.
// TagResource/UntagResource take a context (used elsewhere for endpoint resolution),
// so this uses registerTaggingService directly rather than the ctx-dropping
// wireTaggingARNResources helper.
func wireTaggingECR(bk resourcegroupstaggingapibackend.StorageBackend, ecrReg service.Registerable) {
	ecrH, ok := ecrReg.(*ecrbackend.Handler)
	if !ok {
		return
	}

	ecrBk, ok := ecrH.Backend.(*ecrbackend.InMemoryBackend)
	if !ok {
		return
	}

	registerTaggingService(
		bk,
		func(_ context.Context) []resourcegroupstaggingapibackend.TaggedResource {
			items := ecrBk.TaggedResources()
			out := make([]resourcegroupstaggingapibackend.TaggedResource, 0, len(items))
			for _, item := range items {
				out = append(out, resourcegroupstaggingapibackend.TaggedResource{
					ResourceARN:  item.ARN,
					ResourceType: "ecr:repository",
					Tags:         item.Tags,
				})
			}

			return out
		},
		"ecr",
		ecrBk.TagResource,
		ecrBk.UntagResource,
	)
}

// wireTaggingKinesis wires the Kinesis backend into the Resource Groups Tagging API.
// Kinesis only exposes stream ARNs for tagging (a single resource kind). TagResource/
// UntagResource take *TagResourceInput/*UntagResourceInput rather than bare
// (arn, tags)/(arn, keys) parameters, so this uses registerTaggingService directly.
func wireTaggingKinesis(bk resourcegroupstaggingapibackend.StorageBackend, kinesisReg service.Registerable) {
	kinesisH, ok := kinesisReg.(*kinesisbackend.Handler)
	if !ok {
		return
	}

	kinesisBk, ok := kinesisH.Backend.(*kinesisbackend.InMemoryBackend)
	if !ok {
		return
	}

	registerTaggingService(
		bk,
		func(_ context.Context) []resourcegroupstaggingapibackend.TaggedResource {
			items := kinesisBk.TaggedStreams()
			out := make([]resourcegroupstaggingapibackend.TaggedResource, 0, len(items))
			for _, item := range items {
				out = append(out, resourcegroupstaggingapibackend.TaggedResource{
					ResourceARN:  item.ARN,
					ResourceType: "kinesis:stream",
					Tags:         item.Tags,
				})
			}

			return out
		},
		"kinesis",
		func(ctx context.Context, arn string, newTags map[string]string) error {
			return kinesisBk.TagResource(ctx, &kinesisbackend.TagResourceInput{ResourceARN: arn, Tags: newTags})
		},
		func(ctx context.Context, arn string, keys []string) error {
			return kinesisBk.UntagResource(ctx, &kinesisbackend.UntagResourceInput{ResourceARN: arn, TagKeys: keys})
		},
	)
}

// wireTaggingStepFunctions wires the Step Functions backend into the Resource Groups
// Tagging API. State machines, activities, and state machine aliases share the same
// ARN-keyed tag store on the Handler itself (not the storage backend -- see
// stepfunctions.Handler.TaggedResources), and their ARNs use the "states" service
// namespace (arn:aws:states:{region}:{account}:stateMachine:{name}), not "stepfunctions".
func wireTaggingStepFunctions(bk resourcegroupstaggingapibackend.StorageBackend, sfnReg service.Registerable) {
	sfnH, ok := sfnReg.(*sfnbackend.Handler)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "states",
		func(arn string) string { return resourceTypeFromARN(arn, "states") },
		func() []taggedARNEntry {
			items := sfnH.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		sfnH.TagResourceByARN,
		sfnH.UntagResourceByARN,
	)
}

// wireTaggingCloudFront wires the CloudFront backend into the Resource Groups Tagging
// API. CloudFront keeps tags for every resource kind it supports (distributions,
// streaming distributions, trust stores, distribution tenants, connection groups,
// connection functions, anycast IP lists) behind one taggableTags lookup, so
// resourceTypeFromARN derives the per-resource type instead of a hand-written wiring
// function per kind.
func wireTaggingCloudFront(bk resourcegroupstaggingapibackend.StorageBackend, cfReg service.Registerable) {
	cfH, ok := cfReg.(*cloudfrontbackend.Handler)
	if !ok {
		return
	}

	cfBk := cfH.Backend
	if cfBk == nil {
		return
	}

	wireTaggingARNResources(
		bk, "cloudfront",
		func(arn string) string { return resourceTypeFromARN(arn, "cloudfront") },
		func() []taggedARNEntry {
			items := cfBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		cfBk.TagResource,
		cfBk.UntagResource,
	)
}

// wireTaggingEKS wires the EKS backend into the Resource Groups Tagging API. EKS keeps
// tags for every resource kind it supports (clusters, nodegroups, access entries,
// addons, fargate profiles, pod identity associations, capabilities, Anywhere
// subscriptions) in typed stores searched by resourceARN, so resourceTypeFromARN
// derives the per-resource type instead of a hand-written wiring function per kind.
func wireTaggingEKS(bk resourcegroupstaggingapibackend.StorageBackend, eksReg service.Registerable) {
	eksH, ok := eksReg.(*eksbackend.Handler)
	if !ok {
		return
	}

	eksBk := eksH.Backend
	if eksBk == nil {
		return
	}

	wireTaggingARNResources(
		bk, "eks",
		func(arn string) string { return resourceTypeFromARN(arn, "eks") },
		func() []taggedARNEntry {
			items := eksBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		eksBk.TagResource,
		eksBk.UntagResource,
	)
}

// wireTaggingBatch wires the Batch backend into the Resource Groups Tagging API. Batch
// keeps tags for every resource kind it supports (compute environments, job queues, job
// definitions, jobs, consumable resources, scheduling policies, service environments,
// service jobs) in one flat ARN-keyed lookup, so resourceTypeFromARN derives the
// per-resource type. TagResource/UntagResource take a context (used elsewhere for
// region resolution), so this uses wireTaggingCtxARNResources rather than the
// ctx-dropping wireTaggingARNResources helper.
func wireTaggingBatch(bk resourcegroupstaggingapibackend.StorageBackend, batchReg service.Registerable) {
	batchH, ok := batchReg.(*batchbackend.Handler)
	if !ok {
		return
	}

	batchBk := batchH.Backend
	if batchBk == nil {
		return
	}

	wireTaggingCtxARNResources(
		bk, "batch",
		func(arn string) string { return resourceTypeFromARN(arn, "batch") },
		func() []taggedARNEntry {
			items := batchBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		batchBk.TagResource,
		batchBk.UntagResource,
	)
}

// wireTaggingWAFv2 wires the WAFv2 backend into the Resource Groups Tagging API. WAFv2
// keeps tags for every resource kind it supports (web ACLs, IP sets, regex pattern
// sets, rule groups) behind one lookupTaggedResource lookup, so wafv2ResourceType
// derives the per-resource type (see its doc comment for why resourceTypeFromARN isn't
// enough here). TagResource/UntagResource take a context, so this uses
// wireTaggingCtxARNResources rather than the ctx-dropping wireTaggingARNResources
// helper.
func wireTaggingWAFv2(bk resourcegroupstaggingapibackend.StorageBackend, wafReg service.Registerable) {
	wafH, ok := wafReg.(*wafv2backend.Handler)
	if !ok {
		return
	}

	wafBk, ok := wafH.Backend.(*wafv2backend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingCtxARNResources(
		bk, "wafv2",
		wafv2ResourceType,
		func() []taggedARNEntry {
			items := wafBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		wafBk.TagResource,
		wafBk.UntagResource,
	)
}

// wireTaggingBackup wires the Backup backend into the Resource Groups Tagging API.
// Backup keeps tags for every resource kind it supports (backup vaults, backup plans,
// frameworks, report plans) in typed stores searched by ARN index, so
// resourceTypeFromARN derives the per-resource type instead of a hand-written wiring
// function per kind.
func wireTaggingBackup(bk resourcegroupstaggingapibackend.StorageBackend, backupReg service.Registerable) {
	backupH, ok := backupReg.(*backupbackend.Handler)
	if !ok {
		return
	}

	backupBk := backupH.Backend
	if backupBk == nil {
		return
	}

	wireTaggingARNResources(
		bk, "backup",
		func(arn string) string { return resourceTypeFromARN(arn, "backup") },
		func() []taggedARNEntry {
			items := backupBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		backupBk.TagResource,
		backupBk.UntagResource,
	)
}

// wireTaggingEFS wires the EFS backend into the Resource Groups Tagging API. EFS keeps
// tags for every resource kind it supports (file systems, access points) in typed
// stores, so resourceTypeFromARN derives the per-resource type. EFS ARNs use the
// "elasticfilesystem" service namespace, not "efs". TagResource/UntagResource take a
// context (used elsewhere for region resolution), so this uses wireTaggingCtxARNResources
// rather than the ctx-dropping wireTaggingARNResources helper.
func wireTaggingEFS(bk resourcegroupstaggingapibackend.StorageBackend, efsReg service.Registerable) {
	efsH, ok := efsReg.(*efsbackend.Handler)
	if !ok {
		return
	}

	efsBk := efsH.Backend
	if efsBk == nil {
		return
	}

	wireTaggingCtxARNResources(
		bk, "elasticfilesystem",
		func(arn string) string { return resourceTypeFromARN(arn, "elasticfilesystem") },
		func() []taggedARNEntry {
			items := efsBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		efsBk.TagResource,
		efsBk.UntagResource,
	)
}

// neptuneResourceType derives the resource-type string for a Neptune-owned ARN by
// reading the ARN's own service segment (fields[2]) rather than assuming one, since
// Neptune's single flat tag store spans two ARN services: "neptune" for DB clusters
// and instances (services/neptune/db_clusters.go:70, db_instances.go:32) and "rds" for
// parameter groups, subnet groups, and cluster snapshots
// (services/neptune/cluster_parameter_groups.go:47, subnet_groups.go:41,
// cluster_snapshots.go:44) -- the same "rds" segment the separate RDS and DocumentDB
// backends use.
func neptuneResourceType(arnStr string) string {
	fields := strings.SplitN(arnStr, ":", arnResourceFieldCount)
	if len(fields) != arnResourceFieldCount {
		return "neptune"
	}

	return resourceTypeFromARN(arnStr, fields[2])
}

// wireTaggingDocDB wires the DocumentDB backend into the Resource Groups Tagging API.
// Every DocumentDB resource kind builds its ARN under the "rds" ARN service --
// clusterARN, instanceARN, subnetGroupARN, clusterParameterGroupARN,
// clusterSnapshotARN, globalClusterARN, and eventSubscriptionARN in
// services/docdb/store.go:232-266 all call arn.Build("rds", ...) -- the same service
// segment wireTaggingRDS below already claims for the separate RDS backend. RDS's own
// AddTagsToResource does not validate that an ARN belongs to a resource it actually
// manages (services/rds/tags.go:4-25 stores blindly for any ARN), and
// resourcegroupstaggingapi's RegisterARNTagger tries taggers in registration order,
// stopping at the first one that reports handled=true. So this is wired ahead of
// wireTaggingRDS in wireResourceGroupsTagging (a naive tagger registered after RDS's
// would never run, since RDS's blind claim always wins first), and its tagger/
// untagger only claim an ARN once docdbBk.HasTaggableResource confirms DocDB itself
// has a matching resource, declining (handled=false) otherwise so RDS's tagger gets a
// turn.
func wireTaggingDocDB(bk resourcegroupstaggingapibackend.StorageBackend, docdbReg service.Registerable) {
	docdbH, ok := docdbReg.(*docdbbackend.Handler)
	if !ok {
		return
	}

	docdbBk := docdbH.Backend
	if docdbBk == nil {
		return
	}

	bk.RegisterProvider(func(_ context.Context) []resourcegroupstaggingapibackend.TaggedResource {
		items := docdbBk.TaggedResources()
		out := make([]resourcegroupstaggingapibackend.TaggedResource, 0, len(items))

		for _, item := range items {
			out = append(out, resourcegroupstaggingapibackend.TaggedResource{
				ResourceARN:  item.ARN,
				ResourceType: resourceTypeFromARN(item.ARN, "rds"),
				Tags:         item.Tags,
			})
		}

		return out
	})

	bk.RegisterARNTagger(func(ctx context.Context, arnStr string, newTags map[string]string) (bool, error) {
		if !arnServiceIs(arnStr, "rds") || !docdbBk.HasTaggableResource(ctx, arnStr) {
			return false, nil
		}

		tagList := make([]docdbbackend.Tag, 0, len(newTags))
		for k, v := range newTags {
			tagList = append(tagList, docdbbackend.Tag{Key: k, Value: v})
		}

		return true, docdbBk.AddTagsToResource(ctx, arnStr, tagList)
	})

	bk.RegisterARNUntagger(func(ctx context.Context, arnStr string, keys []string) (bool, error) {
		if !arnServiceIs(arnStr, "rds") || !docdbBk.HasTaggableResource(ctx, arnStr) {
			return false, nil
		}

		docdbBk.RemoveTagsFromResource(ctx, arnStr, keys)

		return true, nil
	})
}

// neptuneOwnsARN reports whether Neptune should claim arnStr for cross-service
// tagging: unconditionally for the "neptune" ARN service (exclusive to Neptune, no
// other wired backend uses it), and only after an ownership check for the "rds" ARN
// service it shares with RDS and DocumentDB. See wireTaggingNeptune.
func neptuneOwnsARN(ctx context.Context, neptuneBk *neptunebackend.InMemoryBackend, arnStr string) bool {
	switch {
	case arnServiceIs(arnStr, "neptune"):
		return true
	case arnServiceIs(arnStr, "rds"):
		return neptuneBk.HasTaggableResource(ctx, arnStr)
	default:
		return false
	}
}

// wireTaggingNeptune wires the Neptune backend into the Resource Groups Tagging API.
// Neptune's single flat tag store spans two ARN services (see neptuneResourceType
// above for the file:line ARN-building evidence): "neptune" for DB clusters and
// instances, which no other wired backend claims, and "rds" for parameter groups,
// subnet groups, and cluster snapshots, which RDS and DocDB also use. The "neptune"
// portion is claimed unconditionally, exactly like every other single-service wiring
// in this file, since arnServiceIs("neptune") alone rules out every other backend. The
// "rds" portion needs the same ownership check as wireTaggingDocDB above and for the
// same reason (RDS's tagger blindly claims any "rds" ARN, and taggers are tried in
// registration order), so this too is wired ahead of wireTaggingRDS and declines
// (handled=false) any "rds" ARN neptuneBk.HasTaggableResource does not recognize.
func wireTaggingNeptune(bk resourcegroupstaggingapibackend.StorageBackend, neptuneReg service.Registerable) {
	neptuneH, ok := neptuneReg.(*neptunebackend.Handler)
	if !ok {
		return
	}

	neptuneBk, ok := neptuneH.Backend.(*neptunebackend.InMemoryBackend)
	if !ok {
		return
	}

	bk.RegisterProvider(func(_ context.Context) []resourcegroupstaggingapibackend.TaggedResource {
		items := neptuneBk.TaggedResources()
		out := make([]resourcegroupstaggingapibackend.TaggedResource, 0, len(items))

		for _, item := range items {
			out = append(out, resourcegroupstaggingapibackend.TaggedResource{
				ResourceARN:  item.ARN,
				ResourceType: neptuneResourceType(item.ARN),
				Tags:         item.Tags,
			})
		}

		return out
	})

	bk.RegisterARNTagger(func(ctx context.Context, arnStr string, newTags map[string]string) (bool, error) {
		if !neptuneOwnsARN(ctx, neptuneBk, arnStr) {
			return false, nil
		}

		tagList := make([]neptunebackend.Tag, 0, len(newTags))
		for k, v := range newTags {
			tagList = append(tagList, neptunebackend.Tag{Key: k, Value: v})
		}

		return true, neptuneBk.AddTagsToResource(ctx, arnStr, tagList)
	})

	bk.RegisterARNUntagger(func(ctx context.Context, arnStr string, keys []string) (bool, error) {
		if !neptuneOwnsARN(ctx, neptuneBk, arnStr) {
			return false, nil
		}

		return true, neptuneBk.RemoveTagsFromResource(ctx, arnStr, keys)
	})
}

// wireTaggingRDS wires the RDS backend into the Resource Groups Tagging API. RDS keeps
// tags for every resource kind it supports (DB instances, clusters, snapshots,
// parameter groups, and more) in one flat ARN-keyed map, so resourceTypeFromARN derives
// the per-resource type. AddTagsToResource/RemoveTagsFromResource take []Tag rather
// than a bare map and return no error, so this adapts them to the
// wireTaggingARNResources (arn, map) error shape.
func wireTaggingRDS(bk resourcegroupstaggingapibackend.StorageBackend, rdsReg service.Registerable) {
	rdsH, ok := rdsReg.(*rdsbackend.Handler)
	if !ok {
		return
	}

	rdsBk := rdsH.Backend
	if rdsBk == nil {
		return
	}

	wireTaggingARNResources(
		bk, "rds",
		func(arn string) string { return resourceTypeFromARN(arn, "rds") },
		func() []taggedARNEntry {
			items := rdsBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		func(arn string, newTags map[string]string) error {
			tagList := make([]rdsbackend.Tag, 0, len(newTags))
			for k, v := range newTags {
				tagList = append(tagList, rdsbackend.Tag{Key: k, Value: v})
			}

			rdsBk.AddTagsToResource(arn, tagList)

			return nil
		},
		func(arn string, keys []string) error {
			rdsBk.RemoveTagsFromResource(arn, keys)

			return nil
		},
	)
}

// wireTaggingElastiCache wires the ElastiCache backend into the Resource Groups
// Tagging API. ElastiCache keeps tags for every resource kind it supports (clusters,
// replication groups, parameter groups, snapshots, security groups, global replication
// groups, subnet groups, serverless caches, users, user groups) behind one
// collectTagCandidatesLocked lookup, so resourceTypeFromARN derives the per-resource
// type. AddTagsToResource/RemoveTagsFromResource take a context, so this uses
// wireTaggingCtxARNResources rather than the ctx-dropping wireTaggingARNResources
// helper.
func wireTaggingElastiCache(bk resourcegroupstaggingapibackend.StorageBackend, ecReg service.Registerable) {
	ecH, ok := ecReg.(*elasticachebackend.Handler)
	if !ok {
		return
	}

	ecBk, ok := ecH.Backend.(*elasticachebackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingCtxARNResources(
		bk, "elasticache",
		func(arn string) string { return resourceTypeFromARN(arn, "elasticache") },
		func() []taggedARNEntry {
			items := ecBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		ecBk.AddTagsToResource,
		ecBk.RemoveTagsFromResource,
	)
}

// redshiftClusterARNFieldCount is the number of colon-delimited fields in a
// well-formed Redshift cluster ARN: "arn:partition:redshift:region:account:cluster:id".
const redshiftClusterARNFieldCount = 7

// redshiftClusterIDFromARN extracts the cluster identifier from a Redshift cluster
// ARN. wireTaggingRedshift's tagger/untagger need this because
// CreateTags/DeleteTags (services/redshift/tags.go) take the bare cluster identifier,
// not an ARN -- see wireTaggingRedshift's doc comment.
func redshiftClusterIDFromARN(arnStr string) (string, bool) {
	parts := strings.SplitN(arnStr, ":", redshiftClusterARNFieldCount)
	if len(parts) != redshiftClusterARNFieldCount || parts[5] != "cluster" {
		return "", false
	}

	return parts[6], true
}

// wireTaggingRedshift wires the Redshift backend into the Resource Groups Tagging
// API. Redshift's own CreateTags/DeleteTags/DescribeTags (services/redshift/tags.go)
// only ever manage cluster tags -- DescribeTags walks b.clusters.All() exclusively,
// per its own doc comment -- and, unlike every other resource wired in this file,
// take a bare cluster identifier rather than an ARN: the handler layer
// (services/redshift/handler_tags.go's handleCreateTags/handleDeleteTags) passes
// ResourceName straight through uninterpreted, so a full ARN would silently look up
// nothing. Cluster ARNs are not built anywhere in this package (Cluster has no ARN
// field of its own; only DescribeTags' request handler
// (handler_tags.go:50's ":cluster:"+clusterID suffix check) encodes the expected
// shape), so this reconstructs it the same way:
// "arn:aws:redshift:{region}:{account}:cluster:{id}". redshift-serverless resources
// (workgroups, namespaces, snapshots, usage limits) use a different ARN service
// ("redshift-serverless", see services/redshift/serverless_workgroups.go:38 and
// siblings) and are not reachable through this same tagging surface, so they are out
// of scope here.
func wireTaggingRedshift(bk resourcegroupstaggingapibackend.StorageBackend, redshiftReg service.Registerable) {
	redshiftH, ok := redshiftReg.(*redshiftbackend.Handler)
	if !ok {
		return
	}

	redshiftBk, ok := redshiftH.Backend.(*redshiftbackend.InMemoryBackend)
	if !ok {
		return
	}

	clusterARN := func(id string) string {
		return arn.Build("redshift", redshiftBk.Region(), redshiftBk.AccountID(), "cluster:"+id)
	}

	wireTaggingARNResources(
		bk, "redshift",
		constantResourceType("redshift:cluster"),
		func() []taggedARNEntry {
			all := redshiftBk.DescribeTags()
			out := make([]taggedARNEntry, 0, len(all))

			for clusterID, tagMap := range all {
				if len(tagMap) == 0 {
					continue
				}

				out = append(out, taggedARNEntry{ARN: clusterARN(clusterID), Tags: tagMap})
			}

			return out
		},
		func(arnStr string, newTags map[string]string) error {
			id, parsed := redshiftClusterIDFromARN(arnStr)
			if !parsed {
				return fmt.Errorf("%w: malformed Redshift cluster ARN: %s", redshiftbackend.ErrClusterNotFound, arnStr)
			}

			return redshiftBk.CreateTags(id, newTags)
		},
		func(arnStr string, keys []string) error {
			id, parsed := redshiftClusterIDFromARN(arnStr)
			if !parsed {
				return fmt.Errorf("%w: malformed Redshift cluster ARN: %s", redshiftbackend.ErrClusterNotFound, arnStr)
			}

			return redshiftBk.DeleteTags(id, keys)
		},
	)
}

// firehoseStreamNameFromARN extracts the delivery stream name from a Firehose ARN
// ("arn:partition:firehose:region:account:deliverystream/name"). Needed because
// TagDeliveryStream/UntagDeliveryStream/ListTagsForDeliveryStream (services/firehose/
// tags.go) take the bare stream name, not an ARN.
func firehoseStreamNameFromARN(arnStr string) (string, bool) {
	const firehoseARNFieldCount = 6

	parts := strings.SplitN(arnStr, ":", firehoseARNFieldCount)
	if len(parts) != firehoseARNFieldCount {
		return "", false
	}

	name, ok := strings.CutPrefix(parts[firehoseARNFieldCount-1], "deliverystream/")
	if !ok {
		return "", false
	}

	return name, true
}

// wireTaggingFirehose wires the Firehose backend into the Resource Groups Tagging
// API. Delivery stream ARNs use the "firehose" ARN service and a "deliverystream/"
// resource prefix (services/firehose/delivery_streams.go:65's
// arn.Build("firehose", region, b.accountID, "deliverystream/"+input.Name) call),
// matching the package name -- no collision with any other wired service. TagDeliveryStream/
// UntagDeliveryStream/ListTagsForDeliveryStream take the bare stream name rather than
// an ARN, so firehoseStreamNameFromARN above recovers it.
func wireTaggingFirehose(bk resourcegroupstaggingapibackend.StorageBackend, firehoseReg service.Registerable) {
	firehoseH, ok := firehoseReg.(*firehosebackend.Handler)
	if !ok {
		return
	}

	firehoseBk, ok := firehoseH.Backend.(*firehosebackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingCtxARNResources(
		bk, "firehose",
		constantResourceType("firehose:deliverystream"),
		func() []taggedARNEntry {
			items := firehoseBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))

			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		func(ctx context.Context, arnStr string, newTags map[string]string) error {
			name, parsed := firehoseStreamNameFromARN(arnStr)
			if !parsed {
				return fmt.Errorf("%w: malformed Firehose delivery stream ARN: %s", firehosebackend.ErrNotFound, arnStr)
			}

			return firehoseBk.TagDeliveryStream(ctx, name, newTags)
		},
		func(ctx context.Context, arnStr string, keys []string) error {
			name, parsed := firehoseStreamNameFromARN(arnStr)
			if !parsed {
				return fmt.Errorf("%w: malformed Firehose delivery stream ARN: %s", firehosebackend.ErrNotFound, arnStr)
			}

			return firehoseBk.UntagDeliveryStream(ctx, name, keys)
		},
	)
}

// wireTaggingOpenSearch wires the OpenSearch backend into the Resource Groups
// Tagging API. AddTags/RemoveTags/ListTags (services/opensearch/tags.go) only ever
// manage domain tags (both look up exclusively via findDomainByARN), and domain ARNs
// use the "es" ARN service -- not "opensearch" -- per
// services/opensearch/domains.go:32's arn.Build("es", b.region, b.accountID,
// "domain/"+input.Name) call (OpenSearch Service domains keep the legacy
// Elasticsearch Service ARN namespace in real AWS too). Applications and direct-query
// data sources use "opensearch" (data_sources.go:75, applications.go:33) but have no
// AddTags/RemoveTags path in this backend, so they are out of scope here.
func wireTaggingOpenSearch(bk resourcegroupstaggingapibackend.StorageBackend, osReg service.Registerable) {
	osH, ok := osReg.(*opensearchbackend.Handler)
	if !ok {
		return
	}

	osBk, ok := osH.Backend.(*opensearchbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "es",
		constantResourceType("es:domain"),
		func() []taggedARNEntry {
			domains, err := osBk.DescribeDomains(nil)
			if err != nil {
				return nil
			}

			out := make([]taggedARNEntry, 0, len(domains))

			for _, d := range domains {
				if d.Tags == nil || d.Tags.Len() == 0 {
					continue
				}

				out = append(out, taggedARNEntry{ARN: d.ARN, Tags: d.Tags.Clone()})
			}

			return out
		},
		osBk.AddTags,
		osBk.RemoveTags,
	)
}

// wireTaggingMQ wires the Amazon MQ backend into the Resource Groups Tagging API. MQ
// keeps tags for both resource kinds it supports (brokers and configurations) in one
// flat ARN-keyed map (services/mq/tags.go), and both use the "mq" ARN service
// (services/mq/brokers.go:170, configurations.go:101), matching the package name --
// no collision with any other wired service. resourceTypeFromARN derives the
// per-resource type ("mq:broker" or "mq:configuration") from each ARN's own resource
// segment.
func wireTaggingMQ(bk resourcegroupstaggingapibackend.StorageBackend, mqReg service.Registerable) {
	mqH, ok := mqReg.(*mqbackend.Handler)
	if !ok {
		return
	}

	mqBk, ok := mqH.Backend.(*mqbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "mq",
		func(arnStr string) string { return resourceTypeFromARN(arnStr, "mq") },
		func() []taggedARNEntry {
			items := mqBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))

			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		mqBk.CreateTags,
		mqBk.DeleteTags,
	)
}

// wireTaggingEMR wires the EMR backend into the Resource Groups Tagging API. EMR
// ARNs use the "elasticmapreduce" ARN service, not "emr"
// (services/emr/clusters.go:207's arn.Build("elasticmapreduce", region, b.accountID,
// "cluster/"+id) call and studios.go:253's matching "studio/"+id call).
// AddTags/RemoveTags (services/emr/tags.go) accept either a cluster identifier or a
// studio ID/ARN and take a context, so this uses wireTaggingCtxARNResources.
func wireTaggingEMR(bk resourcegroupstaggingapibackend.StorageBackend, emrReg service.Registerable) {
	emrH, ok := emrReg.(*emrbackend.Handler)
	if !ok {
		return
	}

	emrBk := emrH.Backend
	if emrBk == nil {
		return
	}

	wireTaggingCtxARNResources(
		bk, "elasticmapreduce",
		func(arnStr string) string { return resourceTypeFromARN(arnStr, "elasticmapreduce") },
		func() []taggedARNEntry {
			items := emrBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))

			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		func(ctx context.Context, arnStr string, newTags map[string]string) error {
			tagList := make([]emrbackend.Tag, 0, len(newTags))
			for k, v := range newTags {
				tagList = append(tagList, emrbackend.Tag{Key: k, Value: v})
			}

			return emrBk.AddTags(ctx, arnStr, tagList)
		},
		emrBk.RemoveTags,
	)
}

// wireTaggingGrafana wires the Amazon Managed Grafana backend into the
// Resource Groups Tagging API. Grafana has exactly one taggable resource
// kind (workspaces), so its resource type is a constant rather than parsed
// per-ARN -- see grafana.InMemoryBackend.WorkspaceARN's doc comment for how
// the "grafana" ARN service and "/workspaces/{id}" resource segment were
// verified (terraform-provider-aws's workspaceARN helper, since the SDK
// itself never emits a workspace ARN on any of its 25 operations).
func wireTaggingGrafana(bk resourcegroupstaggingapibackend.StorageBackend, grafanaReg service.Registerable) {
	grafanaH, ok := grafanaReg.(*grafanabackend.Handler)
	if !ok {
		return
	}

	grafanaBk := grafanaH.Backend
	if grafanaBk == nil {
		return
	}

	wireTaggingCtxARNResources(
		bk, "grafana",
		constantResourceType("grafana:workspace"),
		func() []taggedARNEntry {
			items := grafanaBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))

			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		grafanaBk.TagResource,
		grafanaBk.UntagResource,
	)
}

// wireTaggingOutposts wires the AWS Outposts backend into the Resource Groups
// Tagging API. Unlike Grafana (one taggable resource kind), Outposts has TWO:
// Outpost.Tags and Site.Tags share the same generic ResourceArn-keyed
// TagResource/UntagResource/ListTagsForResource surface (there is no
// dedicated tag API for Sites -- see services/outposts/PARITY.md's "families:
// tagging" note), so resourceTypeFromARN (rather than a single
// constantResourceType) derives "outposts:outpost" or "outposts:site" from
// each ARN's own resource segment. The "outposts" ARN service segment is
// confirmed (not one of the seven service-name mismatches the broader
// campaign found) via in-repo test fixtures in services/ec2 and
// services/route53resolver -- see services/outposts/PARITY.md's ARN section.
func wireTaggingOutposts(bk resourcegroupstaggingapibackend.StorageBackend, outpostsReg service.Registerable) {
	outpostsH, ok := outpostsReg.(*outpostsbackend.Handler)
	if !ok {
		return
	}

	outpostsBk := outpostsH.Backend
	if outpostsBk == nil {
		return
	}

	wireTaggingCtxARNResources(
		bk, "outposts",
		func(arn string) string { return resourceTypeFromARN(arn, "outposts") },
		func() []taggedARNEntry {
			items := outpostsBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))

			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		outpostsBk.TagResource,
		outpostsBk.UntagResource,
	)
}

// wireTaggingResilienceHub wires the AWS Resilience Hub backend into the
// Resource Groups Tagging API. Like Outposts (two taggable resource kinds),
// Resilience Hub has THREE: App, ResiliencyPolicy, and AppAssessment all
// share the same generic ResourceArn-keyed TagResource/UntagResource/
// ListTagsForResource surface (confirmed from every ARN-bearing field's own
// doc comment in aws-sdk-go-v2/service/resiliencehub/types/types.go -- see
// services/resiliencehub/PARITY.md's tagging section), so resourceTypeFromARN
// derives "resiliencehub:app", "resiliencehub:resiliency-policy", or
// "resiliencehub:app-assessment" from each ARN's own resource segment rather
// than hand-writing one wiring function per kind. The "resiliencehub" ARN
// service segment is confirmed directly from those doc comments, not one of
// the seven service-name-mismatch cases the broader campaign found.
func wireTaggingResilienceHub(
	bk resourcegroupstaggingapibackend.StorageBackend,
	resiliencehubReg service.Registerable,
) {
	resiliencehubH, ok := resiliencehubReg.(*resiliencehubbackend.Handler)
	if !ok {
		return
	}

	resiliencehubBk := resiliencehubH.Backend
	if resiliencehubBk == nil {
		return
	}

	wireTaggingCtxARNResources(
		bk, "resiliencehub",
		func(arn string) string { return resourceTypeFromARN(arn, "resiliencehub") },
		func() []taggedARNEntry {
			items := resiliencehubBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))

			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		resiliencehubBk.TagResource,
		resiliencehubBk.UntagResource,
	)
}

// wireTaggingDirectConnect wires the AWS Direct Connect backend into the
// Resource Groups Tagging API. FIVE taggable resource kinds share the one
// "directconnect" ARN namespace -- Connection ("dxcon/"), Lag ("dxlag/"),
// Interconnect (reuses "dxcon/", UNCONFIRMED per PARITY.md),
// VirtualInterface ("dxvif/"), and DirectConnectGateway ("dx-gateway/", a
// GLOBAL ARN with no region segment -- see pkgs/arn.BuildGlobal) -- so this
// uses resourceTypeFromARN's multi-kind dispatch (the same pattern
// wireTaggingOutposts/wireTaggingResilienceHub use for their own two/three
// kinds) rather than one wiring function per kind.
func wireTaggingDirectConnect(
	bk resourcegroupstaggingapibackend.StorageBackend,
	directconnectReg service.Registerable,
) {
	directconnectH, ok := directconnectReg.(*directconnectbackend.Handler)
	if !ok {
		return
	}

	directconnectBk := directconnectH.Backend
	if directconnectBk == nil {
		return
	}

	wireTaggingCtxARNResources(
		bk, "directconnect",
		func(arn string) string { return resourceTypeFromARN(arn, "directconnect") },
		func() []taggedARNEntry {
			items := directconnectBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))

			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		directconnectBk.TagResource,
		directconnectBk.UntagResource,
	)
}

// wireTaggingMGN wires the AWS Application Migration Service backend into the
// Resource Groups Tagging API. 12 taggable resource kinds share the one "mgn"
// ARN namespace (Application, Wave, SourceServer, Job, Connector,
// VcenterClient, LaunchConfigurationTemplate, ReplicationConfigurationTemplate,
// ExportTask, ImportTask, NetworkMigrationDefinition, NetworkMigrationExecution
// -- richer than directconnect's 5 or outposts'/resiliencehub's 2/3), so this
// uses resourceTypeFromARN's multi-kind dispatch, same as
// wireTaggingDirectConnect above.
func wireTaggingMGN(
	bk resourcegroupstaggingapibackend.StorageBackend,
	mgnReg service.Registerable,
) {
	mgnH, ok := mgnReg.(*mgnbackend.Handler)
	if !ok {
		return
	}

	mgnBk := mgnH.Backend
	if mgnBk == nil {
		return
	}

	wireTaggingCtxARNResources(
		bk, "mgn",
		func(arn string) string { return resourceTypeFromARN(arn, "mgn") },
		func() []taggedARNEntry {
			items := mgnBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))

			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		mgnBk.TagResource,
		mgnBk.UntagResource,
	)
}

// wireTaggingNetworkManager wires the AWS Network Manager backend into the
// Resource Groups Tagging API. 9 taggable resource kinds
// (global-network/site/device/link/connection/connect-peer/core-network/
// peering/attachment) share the one "networkmanager" ARN namespace -- all 9
// GLOBAL ARNs with no region segment (confirmed from AWS's own IAM Service
// Authorization Reference, see services/networkmanager/PARITY.md) -- so this
// uses resourceTypeFromARN's multi-kind dispatch, same as wireTaggingMGN/
// wireTaggingDirectConnect above. Unlike mgn, NetworkManager's
// TagResource/UntagResource take no context.Context parameter, so this uses
// wireTaggingARNResources rather than wireTaggingCtxARNResources.
func wireTaggingNetworkManager(
	bk resourcegroupstaggingapibackend.StorageBackend,
	networkmanagerReg service.Registerable,
) {
	networkmanagerH, ok := networkmanagerReg.(*networkmanagerbackend.Handler)
	if !ok {
		return
	}

	networkmanagerBk := networkmanagerH.Backend
	if networkmanagerBk == nil {
		return
	}

	wireTaggingARNResources(
		bk, "networkmanager",
		func(arn string) string { return resourceTypeFromARN(arn, "networkmanager") },
		func() []taggedARNEntry {
			items := networkmanagerBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))

			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		networkmanagerBk.TagResource,
		networkmanagerBk.UntagResource,
	)
}

// wireTaggingLightsail wires the Amazon Lightsail backend into the Resource
// Groups Tagging API. 16 of Lightsail's 20 ResourceType kinds carry a Tags
// field and share the one "lightsail" ARN namespace (see
// services/lightsail/tagging_vpc_misc.go's tagsNotSupportedKinds for the 4
// that do not: StaticIp, PeeredVpc, ExportSnapshotRecord,
// CloudFormationStackRecord). Lightsail's OWN wire-level TagResource/
// UntagResource ops are ResourceName-first (ResourceArn merely optional) --
// the reverse of resourceGroupsTagging's ARN-first convention every other
// wired service here follows (PARITY.md 5.1) -- so this uses the backend's
// TagResourceByARN/UntagResourceByARN adapters (services/lightsail/
// tagging_vpc_misc.go), which resolve an ARN back to the ResourceName the
// real op needs before delegating to the same resolution path, rather than
// wireTaggingCtxARNResources/wireTaggingARNResources needing a
// name-first variant of their own.
func wireTaggingLightsail(
	bk resourcegroupstaggingapibackend.StorageBackend,
	lightsailReg service.Registerable,
) {
	lightsailH, ok := lightsailReg.(*lightsailbackend.Handler)
	if !ok {
		return
	}

	lightsailBk := lightsailH.Backend
	if lightsailBk == nil {
		return
	}

	wireTaggingARNResources(
		bk, "lightsail",
		func(arn string) string { return resourceTypeFromARN(arn, "lightsail") },
		func() []taggedARNEntry {
			items := lightsailBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))

			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		lightsailBk.TagResourceByARN,
		lightsailBk.UntagResourceByARN,
	)
}

// wireTaggingCloudWatchLogs wires the CloudWatch Logs backend into the Resource
// Groups Tagging API. Unlike every other service wired in this file, CloudWatch
// Logs' tag store (h.tags/h.tagsMu) lives on the *Handler itself, not on a Backend --
// see services/cloudwatchlogs/handler.go:29-30 -- so this operates on the Handler
// directly rather than digging into a Backend field. Log group ARNs (and every other
// taggable CloudWatch Logs ARN kind: deliveries, log-anomaly-detectors, lookup
// tables, log streams) use the "logs" ARN service, not "cloudwatchlogs"
// (services/cloudwatchlogs/log_groups.go:22's arn.Build("logs", region, b.accountID,
// "log-group:"+name) call and its siblings in deliveries.go:37, anomaly_detectors.go:47,
// lookup_tables.go:46, log_streams.go:14). The generic "TagResource"/"UntagResource"/
// "ListTagsForResource" actions (handler_tags.go) already key h.tags by the full ARN
// exactly as passed in, so TagResource/UntagResource/GetTagsForResource/
// TaggedResources (added alongside those actions) need no ARN parsing at all.
func wireTaggingCloudWatchLogs(bk resourcegroupstaggingapibackend.StorageBackend, cwlReg service.Registerable) {
	cwlH, ok := cwlReg.(*cwlogsbackend.Handler)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "logs",
		func(arnStr string) string { return resourceTypeFromARN(arnStr, "logs") },
		func() []taggedARNEntry {
			items := cwlH.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))

			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		func(arnStr string, newTags map[string]string) error {
			cwlH.TagResource(arnStr, newTags)

			return nil
		},
		func(arnStr string, keys []string) error {
			cwlH.UntagResource(arnStr, keys)

			return nil
		},
	)
}

// wireTaggingSageMaker wires the SageMaker backend into the Resource Groups Tagging
// API. Every SageMaker ARN this backend builds uses the "sagemaker" ARN service
// (e.g. services/sagemaker/endpoints.go:145, algorithms.go:47, and the many other
// arn.Build("sagemaker", ...) call sites across the package), matching the package
// name -- no collision with any other wired service. AddTags/DeleteTags
// (services/sagemaker/tags.go) already dispatch across every taggable resource kind
// (models, endpoint configs, endpoints, training jobs, notebook instances,
// hyperparameter tuning jobs, actions, algorithms, clusters, model packages,
// processing jobs, transform jobs, domains, feature groups, pipelines, experiments,
// trials, trial components -- see TaggedResources' doc comment for the authoritative
// list) purely by ARN with no resource-kind-specific plumbing needed here, and take a
// context, so this uses wireTaggingCtxARNResources.
func wireTaggingSageMaker(bk resourcegroupstaggingapibackend.StorageBackend, smReg service.Registerable) {
	smH, ok := smReg.(*sagemakerbackend.Handler)
	if !ok {
		return
	}

	smBk := smH.Backend
	if smBk == nil {
		return
	}

	wireTaggingCtxARNResources(
		bk, "sagemaker",
		func(arnStr string) string { return resourceTypeFromARN(arnStr, "sagemaker") },
		func() []taggedARNEntry {
			items := smBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))

			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		smBk.AddTags,
		smBk.DeleteTags,
	)
}

// wireTaggingDAX wires the DAX backend into the Resource Groups Tagging API. DAX only
// assigns ARNs to clusters -- ParameterGroup and SubnetGroup have no Arn field in the
// real SDK types (see InMemoryBackend.arnExists) -- a single resource kind whose ARN
// segment is "cache/{name}" (not "cluster/{name}"), so resourceTypeFromARN derives
// "dax:cache". TagResource/UntagResource return the resulting tag set alongside an
// error, unlike the bare-error shape wireTaggingARNResources expects, so their results
// are adapted away here.
func wireTaggingDAX(bk resourcegroupstaggingapibackend.StorageBackend, daxReg service.Registerable) {
	daxH, ok := daxReg.(*daxbackend.Handler)
	if !ok {
		return
	}

	daxBk, ok := daxH.Backend.(*daxbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "dax",
		func(arn string) string { return resourceTypeFromARN(arn, "dax") },
		func() []taggedARNEntry {
			items := daxBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		func(arn string, tags map[string]string) error {
			_, err := daxBk.TagResource(arn, tags)

			return err
		},
		func(arn string, keys []string) error {
			_, err := daxBk.UntagResource(arn, keys)

			return err
		},
	)
}

// wireTaggingDetective wires the Detective backend into the Resource Groups Tagging
// API. Detective only tags behavior graphs, whose ARN resource segment is
// "graph:{id}" (colon-delimited, not "graph/{id}") -- resourceTypeFromARN handles
// both separators, deriving "detective:graph".
func wireTaggingDetective(bk resourcegroupstaggingapibackend.StorageBackend, detReg service.Registerable) {
	detH, ok := detReg.(*detectivebackend.Handler)
	if !ok {
		return
	}

	detBk, ok := detH.Backend.(*detectivebackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "detective",
		func(arn string) string { return resourceTypeFromARN(arn, "detective") },
		func() []taggedARNEntry {
			items := detBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		detBk.TagResource,
		detBk.UntagResource,
	)
}

// wireTaggingGuardDuty wires the GuardDuty backend into the Resource Groups Tagging
// API. GuardDuty tags detectors and malware protection plans directly ("detector/{id}",
// "malware-protection-plan/{id}") but also filters, IP sets, threat intel/entity sets,
// trusted entity sets, and publishing destinations nested one level under their owning
// detector ("detector/{id}/{kind}/{id}", see InMemoryBackend.syncResourceTagsFromARN)
// -- a plain resourceTypeFromARN would take only the first segment and collide every
// nested kind under "guardduty:detector", so nestedResourceType is used instead.
func wireTaggingGuardDuty(bk resourcegroupstaggingapibackend.StorageBackend, gdReg service.Registerable) {
	gdH, ok := gdReg.(*guarddutybackend.Handler)
	if !ok {
		return
	}

	gdBk, ok := gdH.Backend.(*guarddutybackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "guardduty",
		func(arn string) string { return nestedResourceType(arn, "guardduty") },
		func() []taggedARNEntry {
			items := gdBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		gdBk.TagResource,
		gdBk.UntagResource,
	)
}

// wireTaggingTransfer wires the Transfer Family backend into the Resource Groups
// Tagging API. Most Transfer resource kinds (server, user, connector, certificate,
// profile, webapp, workflow, host-key) put their own kind as the ARN's first segment,
// but agreements nest under their owning server ("server/{id}/agreement/{id}") --
// nestedResourceType handles both shapes, deriving "transfer:agreement" for the
// nested case instead of colliding it under "transfer:server".
func wireTaggingTransfer(bk resourcegroupstaggingapibackend.StorageBackend, xferReg service.Registerable) {
	xferH, ok := xferReg.(*transferbackend.Handler)
	if !ok {
		return
	}

	xferBk, ok := xferH.Backend.(*transferbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "transfer",
		func(arn string) string { return nestedResourceType(arn, "transfer") },
		func() []taggedARNEntry {
			items := xferBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		xferBk.TagResource,
		xferBk.UntagResource,
	)
}

// wireTaggingCognitoIDP wires the Cognito Identity Provider backend into the Resource
// Groups Tagging API. Only user pools are tagged, ARN segment "userpool/{id}", so
// resourceTypeFromARN derives "cognito-idp:userpool" (the real ARN service namespace
// is "cognito-idp", not "cognitoidp"). TagResource/UntagResource have no error return,
// unlike the error-returning shape wireTaggingARNResources expects, so they are
// adapted away here.
func wireTaggingCognitoIDP(bk resourcegroupstaggingapibackend.StorageBackend, idpReg service.Registerable) {
	idpH, ok := idpReg.(*cognitoidpbackend.Handler)
	if !ok {
		return
	}

	idpBk := idpH.Backend
	if idpBk == nil {
		return
	}

	wireTaggingARNResources(
		bk, "cognito-idp",
		func(arn string) string { return resourceTypeFromARN(arn, "cognito-idp") },
		func() []taggedARNEntry {
			items := idpBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		func(arn string, tags map[string]string) error {
			idpBk.TagResource(arn, tags)

			return nil
		},
		func(arn string, keys []string) error {
			idpBk.UntagResource(arn, keys)

			return nil
		},
	)
}

// wireTaggingAppConfig wires the AppConfig backend into the Resource Groups Tagging
// API. Applications, deployment strategies, extensions, and extension associations put
// their own kind as the ARN's first segment, but environments, configuration profiles,
// and experiment definitions nest under their owning application
// ("application/{id}/{kind}/{id}") -- nestedResourceType handles both shapes.
func wireTaggingAppConfig(bk resourcegroupstaggingapibackend.StorageBackend, acReg service.Registerable) {
	acH, ok := acReg.(*appconfigbackend.Handler)
	if !ok {
		return
	}

	acBk, ok := acH.Backend.(*appconfigbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "appconfig",
		func(arn string) string { return nestedResourceType(arn, "appconfig") },
		func() []taggedARNEntry {
			items := acBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		acBk.TagResource,
		acBk.UntagResource,
	)
}

// wireTaggingCodeCommit wires the CodeCommit backend into the Resource Groups Tagging
// API. Only repositories are tagged, and their ARNs carry no type segment at all
// (arn:aws:codecommit:{region}:{account}:{repo-name}, matching real AWS's
// documented ARN format), so this uses a constant resource type -- the same shape
// SQS/SNS use for their own bare-name ARNs -- rather than resourceTypeFromARN, which
// would fall back to just "codecommit" with no ":repository" suffix.
func wireTaggingCodeCommit(bk resourcegroupstaggingapibackend.StorageBackend, ccReg service.Registerable) {
	ccH, ok := ccReg.(*codecommitbackend.Handler)
	if !ok {
		return
	}

	ccBk := ccH.Backend
	if ccBk == nil {
		return
	}

	wireTaggingARNResources(
		bk, "codecommit",
		constantResourceType("codecommit:repository"),
		func() []taggedARNEntry {
			items := ccBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		ccBk.TagResource,
		ccBk.UntagResource,
	)
}

// wireTaggingServiceDiscovery wires the Cloud Map (servicediscovery) backend into the
// Resource Groups Tagging API. Namespaces and services each keep their own kind as
// the ARN's first segment ("namespace/{id}", "service/{id}"), so resourceTypeFromARN
// derives the per-resource type.
func wireTaggingServiceDiscovery(bk resourcegroupstaggingapibackend.StorageBackend, sdReg service.Registerable) {
	sdH, ok := sdReg.(*servicediscoverybackend.Handler)
	if !ok {
		return
	}

	sdBk, ok := sdH.Backend.(*servicediscoverybackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "servicediscovery",
		func(arn string) string { return resourceTypeFromARN(arn, "servicediscovery") },
		func() []taggedARNEntry {
			items := sdBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		sdBk.TagResource,
		sdBk.UntagResource,
	)
}

// wireTaggingMemoryDB wires the MemoryDB backend into the Resource Groups Tagging API.
// Clusters, ACLs, subnet groups, users, parameter groups, and snapshots each keep
// their own kind as the ARN's first segment ("cluster/{name}", "acl/{name}", ...)
// across per-region stores, so resourceTypeFromARN derives the per-resource type.
// TagResource/UntagResource take a context, so this uses wireTaggingCtxARNResources
// rather than the ctx-dropping wireTaggingARNResources helper.
func wireTaggingMemoryDB(bk resourcegroupstaggingapibackend.StorageBackend, mdbReg service.Registerable) {
	mdbH, ok := mdbReg.(*memorydbbackend.Handler)
	if !ok {
		return
	}

	mdbBk, ok := mdbH.Backend.(*memorydbbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingCtxARNResources(
		bk, "memorydb",
		func(arn string) string { return resourceTypeFromARN(arn, "memorydb") },
		func() []taggedARNEntry {
			items := mdbBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		mdbBk.TagResource,
		mdbBk.UntagResource,
	)
}

// wireTaggingAccessAnalyzer wires the IAM Access Analyzer backend into the Resource
// Groups Tagging API. Access Analyzer tags only analyzers ("analyzer/{name}", see
// InMemoryBackend.analyzerARN), a single resource kind, so this uses a constant
// resource type rather than resourceTypeFromARN.
func wireTaggingAccessAnalyzer(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*accessanalyzerbackend.Handler)
	if !ok {
		return
	}

	aaBk, ok := h.Backend.(*accessanalyzerbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "access-analyzer", constantResourceType("access-analyzer:analyzer"),
		func() []taggedARNEntry {
			items := aaBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		aaBk.TagResource,
		aaBk.UntagResource,
	)
}

// wireTaggingDLM wires the DLM backend into the Resource Groups Tagging API. DLM tags
// only lifecycle policies ("policy/{id}", see InMemoryBackend's policyARN callers), a
// single resource kind, so this uses a constant resource type.
func wireTaggingDLM(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*dlmbackend.Handler)
	if !ok {
		return
	}

	dlmBk, ok := h.Backend.(*dlmbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "dlm", constantResourceType("dlm:policy"),
		func() []taggedARNEntry {
			items := dlmBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		dlmBk.TagResource,
		dlmBk.UntagResource,
	)
}

// wireTaggingOpsWorks wires the OpsWorks backend into the Resource Groups Tagging API.
// OpsWorks only accepts stack or layer ARNs (see services/opsworks/tags.go's
// resourceExists), so resourceTypeFromARN correctly derives "opsworks:stack" or
// "opsworks:layer" from each tagged ARN's resource segment.
func wireTaggingOpsWorks(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*opsworksbackend.Handler)
	if !ok {
		return
	}

	opsworksBk, ok := h.Backend.(*opsworksbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "opsworks",
		func(arn string) string { return resourceTypeFromARN(arn, "opsworks") },
		func() []taggedARNEntry {
			items := opsworksBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		opsworksBk.TagResource,
		opsworksBk.UntagResource,
	)
}

// wireTaggingComprehend wires the Comprehend backend into the Resource Groups Tagging
// API. Comprehend keeps tags for every job/resource kind it supports in one flat
// ARN-keyed lookup, so resourceTypeFromARN derives the per-resource type. Confirmed
// against services/comprehend/store.go's resourceARN helper: "comprehend", region,
// account, resourceType+"/"+name(+"/version/"+version for versioned resources, which
// still derives the correct base type since resourceTypeFromARN only reads up to the
// first "/").
//
//nolint:dupl // structurally mirrors wireTaggingAWSConfig (both need mapToTagSlice) but wires an unrelated backend
func wireTaggingComprehend(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*comprehendbackend.Handler)
	if !ok {
		return
	}

	cBk := h.Backend
	if cBk == nil {
		return
	}

	wireTaggingARNResources(
		bk, "comprehend",
		func(arn string) string { return resourceTypeFromARN(arn, "comprehend") },
		func() []taggedARNEntry {
			items := cBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		func(arn string, tags map[string]string) error {
			return cBk.TagResource(arn, mapToTagSlice(tags, func(k, v string) comprehendbackend.Tag {
				return comprehendbackend.Tag{Key: k, Value: v}
			}))
		},
		cBk.UntagResource,
	)
}

// wireTaggingShield wires the Shield backend into the Resource Groups Tagging API.
// TagResource/ListTagsForResource/UntagResource only resolve Shield protection ARNs or
// resource ARNs already associated with a protection (see resolveTaggableProtection in
// services/shield/tags.go) -- protection groups build their own ARN
// (protectionGroupARN) but are never accepted by TagResource, so protections are the
// only taggable kind and a constant resource type is used, matching this repo's
// SQS/SNS-style single-kind convention.
func wireTaggingShield(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*shieldbackend.Handler)
	if !ok {
		return
	}

	sBk, ok := h.Backend.(*shieldbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "shield", constantResourceType("shield:protection"),
		func() []taggedARNEntry {
			items := sBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		sBk.TagResource,
		sBk.UntagResource,
	)
}

// wireTaggingTranscribe wires the Transcribe backend into the Resource Groups Tagging
// API. Transcribe keeps tags for every job/vocabulary/model kind it supports in one flat
// ARN-keyed lookup, so resourceTypeFromARN derives the per-resource type. Confirmed
// against services/transcribe/store.go's resourceARN helper: "transcribe",
// config.DefaultRegion, defaultAccountID, resourceType+"/"+name.
func wireTaggingTranscribe(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*transcribebackend.Handler)
	if !ok {
		return
	}

	tBk, ok := h.Backend.(*transcribebackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "transcribe",
		func(arn string) string { return resourceTypeFromARN(arn, "transcribe") },
		func() []taggedARNEntry {
			items := tBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		tBk.TagResource,
		tBk.UntagResource,
	)
}

// wireTaggingVerifiedPermissions wires the Verified Permissions backend into the
// Resource Groups Tagging API. It keeps tags for policy stores, policies, policy
// templates, and identity sources in one flat ARN-keyed lookup, so resourceTypeFromARN
// derives the per-resource type. Confirmed against services/verifiedpermissions'
// arn.Build call sites: "verifiedpermissions", region (sometimes ""), account,
// resourceType+"/"+resourceID.
func wireTaggingVerifiedPermissions(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*verifiedpermissionsbackend.Handler)
	if !ok {
		return
	}

	vpBk, ok := h.Backend.(*verifiedpermissionsbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "verifiedpermissions",
		func(arn string) string { return resourceTypeFromARN(arn, "verifiedpermissions") },
		func() []taggedARNEntry {
			items := vpBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		vpBk.TagResource,
		vpBk.UntagResource,
	)
}

// wireTaggingWAF wires the WAF Classic backend into the Resource Groups Tagging API.
// WAF keeps tags for IP sets, rules, rule groups, rate-based rules, and web ACLs in one
// flat ARN-keyed lookup, so resourceTypeFromARN derives the per-resource type. Confirmed
// against services/waf's arn.Build call sites: "waf", "" (global, no region), account,
// kind+"/"+id.
func wireTaggingWAF(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*wafbackend.Handler)
	if !ok {
		return
	}

	wBk, ok := h.Backend.(*wafbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "waf",
		func(arn string) string { return resourceTypeFromARN(arn, "waf") },
		func() []taggedARNEntry {
			items := wBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		wBk.TagResource,
		wBk.UntagResource,
	)
}

// wireTaggingSecurityHub wires the Security Hub backend into the Resource Groups
// Tagging API. It keeps tags for every resource kind it supports (hubs, standards
// subscriptions, configuration policies, aggregators, connectors, action targets,
// finding aggregators, and more) in one flat ARN-keyed lookup, so resourceTypeFromARN
// derives the per-resource type. Confirmed against services/securityhub's arn.Build
// call sites: "securityhub", region, account, kind+"/"+id (fmt.Sprintf-built, same
// "type/id" shape).
func wireTaggingSecurityHub(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*securityhubbackend.Handler)
	if !ok {
		return
	}

	shBk, ok := h.Backend.(*securityhubbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "securityhub",
		func(arn string) string { return resourceTypeFromARN(arn, "securityhub") },
		func() []taggedARNEntry {
			items := shBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		shBk.TagResource,
		shBk.UntagResource,
	)
}

// wireTaggingAppRunner wires the App Runner backend into the Resource Groups Tagging
// API. It keeps tags for services, auto-scaling configurations, connections,
// observability configurations, VPC connectors, and VPC ingress connections in one flat
// ARN-keyed lookup; resourceExists (services/apprunner/tags.go) accepts all six kinds,
// so resourceTypeFromARN derives the per-resource type from each ARN's own leading
// segment (e.g. "autoscalingconfiguration/{name}/{revision}/{id}" still yields
// "apprunner:autoscalingconfiguration" -- resourceTypeFromARN reads only up to the
// first "/", so the extra revision/id segments of this single-kind compound key don't
// need nestedResourceType).
func wireTaggingAppRunner(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*apprunnerbackend.Handler)
	if !ok {
		return
	}

	arBk, ok := h.Backend.(*apprunnerbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "apprunner",
		func(arn string) string { return resourceTypeFromARN(arn, "apprunner") },
		func() []taggedARNEntry {
			items := arBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		arBk.TagResource,
		arBk.UntagResource,
	)
}

// wireTaggingRoute53Resolver wires the Route 53 Resolver backend into the Resource
// Groups Tagging API. It keeps tags for every resource kind it supports (resolver
// endpoints, resolver rules, resolver configs, firewall domain lists, firewall rules,
// firewall rule groups, firewall rule group associations, outpost resolvers, and query
// log configs) in one flat ARN-keyed lookup with no nesting -- every kind is its own
// top-level "type/id" segment (confirmed against every arn.Build call site in the
// package) -- so resourceTypeFromARN derives the per-resource type.
// TagResource/UntagResource take a context (used for region resolution), so this uses
// wireTaggingCtxARNResources.
func wireTaggingRoute53Resolver(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*route53resolverbackend.Handler)
	if !ok {
		return
	}

	r53Bk, ok := h.Backend.(*route53resolverbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingCtxARNResources(
		bk, "route53resolver",
		func(arn string) string { return resourceTypeFromARN(arn, "route53resolver") },
		func() []taggedARNEntry {
			items := r53Bk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		func(ctx context.Context, arn string, tags map[string]string) error {
			return r53Bk.TagResource(ctx, arn, svctags.MapToKV(tags))
		},
		r53Bk.UntagResource,
	)
}

// wireTaggingTimestreamWrite wires the Timestream Write backend into the Resource
// Groups Tagging API. Its ARN service namespace is "timestream", not "timestreamwrite"
// (confirmed against services/timestreamwrite/store.go's databaseARN/tableARN
// helpers). Tables nest one level under their database
// ("database/{db}/table/{table}"), which nestedResourceType handles (a bare database
// ARN falls back to resourceTypeFromARN's first-segment rule).
func wireTaggingTimestreamWrite(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*timestreamwritebackend.Handler)
	if !ok {
		return
	}

	twBk := h.Backend
	if twBk == nil {
		return
	}

	wireTaggingARNResources(
		bk, "timestream",
		func(arn string) string { return nestedResourceType(arn, "timestream") },
		func() []taggedARNEntry {
			items := twBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		twBk.TagResource,
		twBk.UntagResource,
	)
}

// wireTaggingS3Tables wires the S3 Tables backend into the Resource Groups Tagging
// API. It tags both table bucket ARNs and table ARNs, which nest a table under a
// namespace under a bucket (see s3tablesResourceType's doc comment for why neither
// resourceTypeFromARN nor nestedResourceType fits this depth).
func wireTaggingS3Tables(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*s3tablesbackend.Handler)
	if !ok {
		return
	}

	stBk := h.Backend
	if stBk == nil {
		return
	}

	wireTaggingARNResources(
		bk, "s3tables", s3tablesResourceType,
		func() []taggedARNEntry {
			items := stBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		stBk.TagResource,
		stBk.UntagResource,
	)
}

// wireTaggingS3 wires the S3 backend into the Resource Groups Tagging API.
// Bucket ARNs (arn:aws:s3:::name) share the "s3" ARN service token with S3
// Control's own resources (access points, jobs, access grants -- see
// services/s3control/store.go's arnFmt* constants), so s3OwnsARN is used
// instead of a plain arnServiceIs check to avoid claiming those (gopherstack-8kco).
func wireTaggingS3(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*s3backend.S3Handler)
	if !ok {
		return
	}

	s3Bk, ok := h.Backend.(*s3backend.InMemoryBackend)
	if !ok {
		return
	}

	bk.RegisterProvider(func(_ context.Context) []resourcegroupstaggingapibackend.TaggedResource {
		items := s3Bk.TaggedResources()
		out := make([]resourcegroupstaggingapibackend.TaggedResource, 0, len(items))

		for _, item := range items {
			out = append(out, resourcegroupstaggingapibackend.TaggedResource{
				ResourceARN:  item.ARN,
				ResourceType: "s3:bucket",
				Tags:         item.Tags,
			})
		}

		return out
	})

	bk.RegisterARNTagger(func(_ context.Context, arnStr string, newTags map[string]string) (bool, error) {
		if !s3OwnsARN(arnStr) {
			return false, nil
		}

		return true, s3Bk.MergeBucketTags(s3BucketNameFromARN(arnStr), newTags)
	})

	bk.RegisterARNUntagger(func(_ context.Context, arnStr string, keys []string) (bool, error) {
		if !s3OwnsARN(arnStr) {
			return false, nil
		}

		return true, s3Bk.RemoveBucketTags(s3BucketNameFromARN(arnStr), keys)
	})
}

// s3OwnsARN reports whether arnStr is a plain S3 bucket ARN (arn:aws:s3:::name)
// rather than one of S3 Control's own resources under the shared "s3" ARN
// service token. S3 bucket names can never contain "/" (bucket naming rules),
// while every S3 Control resource kind nests a "kind/id" segment (accesspoint/,
// job/, access-grants/, ... -- see services/s3control/store.go's arnFmt*
// constants), so an unslashed resource segment is unambiguously a bucket.
func s3OwnsARN(arnStr string) bool {
	if !arnServiceIs(arnStr, "s3") {
		return false
	}

	parts := strings.SplitN(arnStr, ":", arnResourceFieldCount)

	return len(parts) == arnResourceFieldCount && !strings.Contains(parts[arnResourceFieldCount-1], "/")
}

// s3BucketNameFromARN extracts the bucket name from a bucket ARN
// (arn:aws:s3:::name). Callers must confirm s3OwnsARN first.
func s3BucketNameFromARN(arnStr string) string {
	parts := strings.SplitN(arnStr, ":", arnResourceFieldCount)
	if len(parts) != arnResourceFieldCount {
		return ""
	}

	return parts[arnResourceFieldCount-1]
}

// wireTaggingS3Control wires the S3 Control backend into the Resource Groups
// Tagging API, covering only the resource kinds taggable through its generic
// TagResource/UntagResource/ListTagsForResource ops (access points, Object
// Lambda access points, multi-region access points, access grants -- see
// InMemoryBackend.TaggedResources' doc comment). Batch job tags and Storage
// Lens configuration tags are real, separate AWS ops
// (Put/Get/DeleteJobTagging, Put/GetStorageLensConfigurationTagging) with no
// generic ARN-tagger equivalent, and Outposts bucket tags live in their own
// name-keyed store (bucketTagging, not ARN-keyed) -- none of the three fit
// this dispatch shape and remain unwired.
func wireTaggingS3Control(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*s3controlbackend.Handler)
	if !ok {
		return
	}

	s3cBk := h.Backend
	if s3cBk == nil {
		return
	}

	bk.RegisterProvider(func(_ context.Context) []resourcegroupstaggingapibackend.TaggedResource {
		items := s3cBk.TaggedResources()
		out := make([]resourcegroupstaggingapibackend.TaggedResource, 0, len(items))

		for _, item := range items {
			out = append(out, resourcegroupstaggingapibackend.TaggedResource{
				ResourceARN:  item.ARN,
				ResourceType: s3controlResourceType(item.ARN),
				Tags:         item.Tags,
			})
		}

		return out
	})

	bk.RegisterARNTagger(func(_ context.Context, arnStr string, newTags map[string]string) (bool, error) {
		if !s3controlOwnsARN(arnStr) {
			return false, nil
		}

		s3cBk.TagResource(arnStr, newTags)

		return true, nil
	})

	bk.RegisterARNUntagger(func(_ context.Context, arnStr string, keys []string) (bool, error) {
		if !s3controlOwnsARN(arnStr) {
			return false, nil
		}

		s3cBk.UntagResource(arnStr, keys)

		return true, nil
	})
}

// s3controlOwnsARN reports whether arnStr is one of S3 Control's generically
// taggable resources (see wireTaggingS3Control): Object Lambda access points
// use the "s3-object-lambda" ARN service token outright, while access points
// and access grants share the "s3" token with S3 bucket ARNs -- see
// s3OwnsARN's doc comment for why requiring a nested "kind/id" resource
// segment distinguishes them.
func s3controlOwnsARN(arnStr string) bool {
	if arnServiceIs(arnStr, "s3-object-lambda") {
		return true
	}

	if !arnServiceIs(arnStr, "s3") {
		return false
	}

	parts := strings.SplitN(arnStr, ":", arnResourceFieldCount)

	return len(parts) == arnResourceFieldCount && strings.Contains(parts[arnResourceFieldCount-1], "/")
}

// s3controlResourceType derives the AWS resource-type string for an s3control
// ARN (e.g. "s3:accesspoint", "s3-object-lambda:accesspoint") from the ARN's
// own service field, since s3controlOwnsARN accepts ARNs from two different
// ARN service tokens.
func s3controlResourceType(resourceARN string) string {
	parts := strings.SplitN(resourceARN, ":", arnResourceFieldCount)
	if len(parts) != arnResourceFieldCount {
		return "s3control"
	}

	return resourceTypeFromARN(resourceARN, parts[2])
}

// wireTaggingAppSync wires the AppSync backend into the Resource Groups Tagging API.
// AppSync's own generic TagResource/UntagResource take an apiId, not the resourceArn
// the tagging aggregator deals in (see appsync.InMemoryBackend.TagResource's doc
// comment), so appsyncAPIIDFromARN bridges the two.
func wireTaggingAppSync(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*appsyncbackend.Handler)
	if !ok {
		return
	}

	apBk, ok := h.Backend.(*appsyncbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "appsync",
		func(arn string) string { return resourceTypeFromARN(arn, "appsync") },
		func() []taggedARNEntry {
			items := apBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		func(arnStr string, newTags map[string]string) error {
			return apBk.TagResource(appsyncAPIIDFromARN(arnStr), newTags)
		},
		func(arnStr string, keys []string) error {
			return apBk.UntagResource(appsyncAPIIDFromARN(arnStr), keys)
		},
	)
}

// appsyncAPIIDFromARN extracts the apiId from an AppSync resource ARN
// (arn:aws:appsync:region:account:apis/{apiId}). Both GraphqlAPI (v1) and
// Api (v2 Event API) resources share this ARN shape (see
// appsync.InMemoryBackend.TagResource's doc comment).
func appsyncAPIIDFromARN(arnStr string) string {
	parts := strings.SplitN(arnStr, ":", arnResourceFieldCount)
	if len(parts) != arnResourceFieldCount {
		return ""
	}

	const apisPrefix = "apis/"

	resource := parts[arnResourceFieldCount-1]
	if !strings.HasPrefix(resource, apisPrefix) {
		return ""
	}

	return resource[len(apisPrefix):]
}

// wireTaggingEmrServerless wires the EMR Serverless backend into the Resource Groups
// Tagging API, covering applications and job runs -- see
// emrserverless.InMemoryBackend.TaggedResources' doc comment for why sessions are
// excluded.
func wireTaggingEmrServerless(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*emrserverlessbackend.Handler)
	if !ok {
		return
	}

	emrBk := h.Backend
	if emrBk == nil {
		return
	}

	wireTaggingARNResources(
		bk, "emr-serverless", emrServerlessResourceType,
		func() []taggedARNEntry {
			items := emrBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		emrBk.TagResource,
		emrBk.UntagResource,
	)
}

// emrServerlessResourceType derives the resource-type string for an EMR Serverless
// ARN. Its resource segment leads with a "/" ("/applications/{id}" or
// "/applications/{id}/jobruns/{id}" -- see InMemoryBackend.applicationARN/jobRunARN),
// unlike the "type/id" shape resourceTypeFromARN/nestedResourceType expect, so this
// strips it before reading segments.
func emrServerlessResourceType(resourceARN string) string {
	parts := strings.SplitN(resourceARN, ":", arnResourceFieldCount)
	if len(parts) != arnResourceFieldCount {
		return "emr-serverless"
	}

	segs := strings.Split(strings.TrimPrefix(parts[arnResourceFieldCount-1], "/"), "/")

	switch {
	case len(segs) == nestedResourceARNSegmentCount:
		return "emr-serverless:" + segs[2]
	case len(segs) >= emrServerlessFlatResourceSegmentCount:
		return "emr-serverless:" + segs[0]
	default:
		return "emr-serverless"
	}
}

// emrServerlessFlatResourceSegmentCount is the minimum "/"-delimited segment count
// ("applications", "{id}") of a flat, non-nested EMR Serverless resource ARN.
const emrServerlessFlatResourceSegmentCount = 2

// wireTaggingACM wires the ACM backend into the Resource Groups Tagging API, covering
// every resource kind its own generic TagResource/UntagResource/ListTagsForResource ops
// accept (see acm.Handler.resolveTaggableResourceArn's doc comment): certificate,
// acme-endpoint, acme-external-account-binding, acme-domain-validation. acme-account is
// excluded -- it has no Tags field and no create-via-API path (botocore 1.43.56,
// acm/2015-12-08/service-2.json.gz has no CreateAcmeAccount operation), matching real ACM.
// Certificate, acme-endpoint, and its two nested kinds all share the "acm" ARN service
// token; nestedResourceType tells the flat kinds (certificate/{id}, acme-endpoint/{id})
// apart from the two nested under an owning endpoint (acme-endpoint/{epId}/acme-.../{id}).
func wireTaggingACM(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*acmbackend.Handler)
	if !ok {
		return
	}

	wireTaggingCtxARNResources(
		bk, "acm",
		func(arnStr string) string { return nestedResourceType(arnStr, "acm") },
		func() []taggedARNEntry {
			items := h.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		h.TagResource,
		h.UntagResource,
	)
}

// wireTaggingSSOAdmin wires the SSO Admin backend into the Resource Groups Tagging API,
// covering the four resource kinds its own TaggableResourceArn pattern documents
// (botocore 1.43.56, sso-admin/2020-07-20/service-2.json.gz): instance, permission set,
// application, trusted token issuer. SSO Admin's own TagResource/UntagResource are keyed
// by (instanceArn, resourceArn), not the bare resourceArn the tagging aggregator deals
// in, so the tag/untag closures below call InstanceArnForResource first to bridge the two.
func wireTaggingSSOAdmin(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*ssoadminbackend.Handler)
	if !ok {
		return
	}

	ssoBk, ok := h.Backend.(*ssoadminbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "sso",
		func(arnStr string) string { return resourceTypeFromARN(arnStr, "sso") },
		func() []taggedARNEntry {
			items := ssoBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		func(arnStr string, newTags map[string]string) error {
			instanceArn, found := ssoBk.InstanceArnForResource(arnStr)
			if !found {
				return ssoadminbackend.ErrInstanceNotFound
			}

			return ssoBk.TagResource(instanceArn, arnStr, newTags)
		},
		func(arnStr string, keys []string) error {
			instanceArn, found := ssoBk.InstanceArnForResource(arnStr)
			if !found {
				return ssoadminbackend.ErrInstanceNotFound
			}

			return ssoBk.UntagResource(instanceArn, arnStr, keys)
		},
	)
}

// apigwStageARNSegs is the "/"-delimited segment count of a nested API Gateway
// stage ARN's resource path ("restapis/{id}/stages/{name}"), the only nested
// kind among API Gateway's seven taggable resources (see
// services/apigateway/tags.go's resolveTaggableARN).
const apigwStageARNSegs = 4

// apigatewayResourceType derives the resource-type string for an API Gateway
// ARN. API Gateway ARNs carry no account segment and their resource portion is
// a leading-slash path ("/restapis/{id}[/stages/{name}]"), which is exactly
// what GetResources documents each resource type as embedding -- so neither
// resourceTypeFromARN (its first "/"-or-":" split would read the leading slash
// itself as the type) nor nestedResourceType fits; this mirrors
// resolveTaggableARN's own "::" + "/" parsing instead.
func apigatewayResourceType(resourceARN string) string {
	_, path, ok := strings.Cut(resourceARN, "::")
	if !ok {
		return "apigateway"
	}

	segs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(segs) == 0 || segs[0] == "" {
		return "apigateway"
	}

	if len(segs) >= apigwStageARNSegs && segs[0] == "restapis" && segs[2] == "stages" {
		return "apigateway:restapis/stages"
	}

	return "apigateway:" + segs[0]
}

// wireTaggingAPIGateway wires the API Gateway backend into the Resource Groups
// Tagging API, covering all seven resource kinds its own generic
// TagResource/UntagResource/GetResourceTags ops accept (RestApi, Stage, ApiKey,
// DomainName, UsagePlan, VpcLink, ClientCertificate -- confirmed against the
// AWS-documented taggable resource list at
// https://docs.aws.amazon.com/apigateway/latest/developerguide/
// apigateway-tagging-supported-resources.html, and against botocore 1.43.56
// apigateway/2015-07-09/service-2.json.gz's RestApi/Stage/ApiKey/DomainName/
// UsagePlan/VpcLink/ClientCertificate shapes, all of which carry a tags
// member). Stages nest under their owning REST API's ARN
// ("/restapis/{id}/stages/{name}"), hence apigatewayResourceType rather than a
// plain first-segment split.
func wireTaggingAPIGateway(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*apigwbackend.Handler)
	if !ok {
		return
	}

	apigwBk, ok := h.Backend.(*apigwbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "apigateway",
		apigatewayResourceType,
		func() []taggedARNEntry {
			items := apigwBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		apigwBk.TagResource,
		apigwBk.UntagResource,
	)
}

// wireTaggingOrganizations wires the Organizations backend into the Resource
// Groups Tagging API, covering the four resource kinds real TagResource
// documents (account, root, organizational unit, policy -- see the AWS
// Organizations API reference for TagResource: its ResourceId pattern has no
// branch for an organization ID). The organization resource itself is
// deliberately excluded even though gopherstack's own local TagResource
// permits tagging it (services/organizations/store.go's
// resourceExistsLocked treats b.org.ID as valid) -- real AWS Organizations has
// no way to tag the organization resource. Organizations' own tag store is
// keyed by an internal resourceID, not an ARN (services/organizations/
// tags.go), so TagResourceByARN/UntagResourceByARN/TaggedResources bridge the
// two; every stored resource already carries its own ARN precomputed at
// creation time, so no new ARN-building logic was needed here.
func wireTaggingOrganizations(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*organizationsbackend.Handler)
	if !ok {
		return
	}

	orgBk, ok := h.Backend.(*organizationsbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "organizations",
		func(arnStr string) string { return resourceTypeFromARN(arnStr, "organizations") },
		func() []taggedARNEntry {
			items := orgBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		orgBk.TagResourceByARN,
		orgBk.UntagResourceByARN,
	)
}

// wireTaggingWorkMail wires the WorkMail backend into the Resource Groups Tagging API.
// Organizations are flat ("organization/{id}") but users, groups, and resources nest
// one level under their owning organization ("organization/{id}/{user|group|resource}/
// {id}", confirmed against services/workmail/store.go's entityARN helper), so
// nestedResourceType derives the per-resource type.
func wireTaggingWorkMail(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*workmailbackend.Handler)
	if !ok {
		return
	}

	wmBk, ok := h.Backend.(*workmailbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "workmail",
		func(arn string) string { return nestedResourceType(arn, "workmail") },
		func() []taggedARNEntry {
			items := wmBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		func(arn string, tags map[string]string) error {
			return wmBk.TagResource(arn, mapToTagSlice(tags, func(k, v string) workmailbackend.Tag {
				return workmailbackend.Tag{Key: k, Value: v}
			}))
		},
		wmBk.UntagResource,
	)
}

// wireTaggingPinpoint wires the Pinpoint backend into the Resource Groups Tagging API.
// Its ARN service namespace is "mobiletargeting", not "pinpoint" (confirmed against
// every arn.Build call site in services/pinpoint). Apps and templates are flat, but
// campaigns/journeys/segments nest one level under their owning app
// ("apps/{appId}/{campaigns|journeys|segments}/{id}"), so nestedResourceType derives
// the per-resource type. Export/import jobs also build "apps/..." ARNs but are never
// entered into the tag index (see TaggedResources' doc comment), so they never surface.
func wireTaggingPinpoint(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*pinpointbackend.Handler)
	if !ok {
		return
	}

	pBk, ok := h.Backend.(*pinpointbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "mobiletargeting",
		func(arn string) string { return nestedResourceType(arn, "mobiletargeting") },
		func() []taggedARNEntry {
			items := pBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		pBk.TagResource,
		pBk.UntagResource,
	)
}

// wireTaggingApplicationAutoScaling wires the Application Auto Scaling backend into
// the Resource Groups Tagging API. Scalable targets build ARNs under the real
// "application-autoscaling" namespace (services/applicationautoscaling/
// scalable_targets.go), while scheduled actions and scaling policies build ARNs under
// "autoscaling" instead (matching real AWS's own historical split) -- but
// TagResource/ListTagsForResource/UntagResource only ever resolve scalable targets
// (see TaggedResources' doc comment), so only the "application-autoscaling" namespace
// is wired, with a constant resource type since it is the only taggable kind.
func wireTaggingApplicationAutoScaling(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*applicationautoscalingbackend.Handler)
	if !ok {
		return
	}

	aasBk := h.Backend
	if aasBk == nil {
		return
	}

	wireTaggingARNResources(
		bk, "application-autoscaling",
		constantResourceType("application-autoscaling:scalable-target"),
		func() []taggedARNEntry {
			items := aasBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		aasBk.TagResource,
		aasBk.UntagResource,
	)
}

// wireTaggingCodeArtifact wires the CodeArtifact backend into the Resource Groups
// Tagging API. Domains, repositories, and package groups are each their own top-level
// "type/id"-shaped ARN (confirmed against every arn.Build call site in the package,
// including repository's "repository/{domain}/{repo}" and package-group's
// "package-group/{domain}{pattern}" -- both still resolve correctly since
// resourceTypeFromARN reads only up to the first "/"), so resourceTypeFromARN derives
// the per-resource type. TagResource/UntagResource take a context (used for region
// resolution), so this uses wireTaggingCtxARNResources.
func wireTaggingCodeArtifact(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*codeartifactbackend.Handler)
	if !ok {
		return
	}

	caBk := h.Backend
	if caBk == nil {
		return
	}

	wireTaggingCtxARNResources(
		bk, "codeartifact",
		func(arn string) string { return resourceTypeFromARN(arn, "codeartifact") },
		func() []taggedARNEntry {
			items := caBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		caBk.TagResource,
		caBk.UntagResource,
	)
}

// wireTaggingCleanRooms wires the Clean Rooms backend into the Resource Groups
// Tagging API. Collaborations, memberships, and configured tables are flat, but
// configured-table associations, ID namespace associations, configured-audience-model
// associations, analysis templates, privacy budget templates, ID mapping tables, and
// intermediate tables all nest one level under their owning membership
// ("membership/{id}/{kind}/{id}", confirmed against every arn.Build call site in the
// package), so nestedResourceType derives the per-resource type.
func wireTaggingCleanRooms(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*cleanroomsbackend.Handler)
	if !ok {
		return
	}

	crBk, ok := h.Backend.(*cleanroomsbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "cleanrooms",
		func(arn string) string { return nestedResourceType(arn, "cleanrooms") },
		func() []taggedARNEntry {
			items := crBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		crBk.TagResource,
		crBk.UntagResource,
	)
}

// wireTaggingAppMesh wires the App Mesh backend into the Resource Groups Tagging API.
// See appmeshResourceType's doc comment for why mesh sub-resources need their own
// derivation rather than resourceTypeFromARN or nestedResourceType.
func wireTaggingAppMesh(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*appmeshbackend.Handler)
	if !ok {
		return
	}

	amBk, ok := h.Backend.(*appmeshbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "appmesh", appmeshResourceType,
		func() []taggedARNEntry {
			items := amBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		amBk.TagResource,
		amBk.UntagResource,
	)
}

// wireTaggingPersonalize wires the Personalize backend into the Resource Groups
// Tagging API. It keeps tags for every resource kind it supports (dataset groups,
// datasets, schemas, solutions, solution versions, campaigns, event trackers, filters,
// recommenders, metric attributions, and jobs) in one flat ARN-keyed lookup, each its
// own top-level "type/id" segment (confirmed against services/personalize/store.go's
// personalizeARN helper), so resourceTypeFromARN derives the per-resource type.
func wireTaggingPersonalize(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*personalizebackend.Handler)
	if !ok {
		return
	}

	pzBk := h.Backend
	if pzBk == nil {
		return
	}

	wireTaggingARNResources(
		bk, "personalize",
		func(arn string) string { return resourceTypeFromARN(arn, "personalize") },
		func() []taggedARNEntry {
			items := pzBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		pzBk.TagResource,
		pzBk.UntagResource,
	)
}

// wireTaggingSESv2 wires the SESv2 backend into the Resource Groups Tagging API. Its
// ARN service namespace is "ses", not "sesv2" (confirmed against
// services/sesv2/deliverability.go and tenants.go's arn.Build call sites -- SESv2
// identities and tenants share the same "ses" namespace SES v1 uses for the same
// underlying resources). Identities and tenants are each their own flat "type/id" ARN,
// so resourceTypeFromARN derives the per-resource type. services/ses (SES v1) builds
// no ARNs of its own today, so there is no shared-namespace ownership collision to
// guard against yet -- unlike kinesisanalytics v1/v2, see this issue's notes.
func wireTaggingSESv2(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*sesv2backend.Handler)
	if !ok {
		return
	}

	sesBk, ok := h.Backend.(*sesv2backend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "ses",
		func(arn string) string { return resourceTypeFromARN(arn, "ses") },
		func() []taggedARNEntry {
			items := sesBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		sesBk.TagResource,
		sesBk.UntagResource,
	)
}

// wireTaggingXRay wires the X-Ray backend into the Resource Groups Tagging API.
// resourceExists (services/xray/tags.go) only accepts group and sampling-rule ARNs,
// each its own flat "type/.../id" segment (group ARNs carry a hardcoded "default"
// scope segment, "group/default/{name}", but resourceTypeFromARN still derives
// "xray:group" correctly since it reads only up to the first "/"), so
// resourceTypeFromARN derives the per-resource type.
func wireTaggingXRay(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*xraybackend.Handler)
	if !ok {
		return
	}

	xBk, ok := h.Backend.(*xraybackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "xray",
		func(arn string) string { return resourceTypeFromARN(arn, "xray") },
		func() []taggedARNEntry {
			items := xBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		xBk.TagResource,
		xBk.UntagResource,
	)
}

// wireTaggingAWSConfig wires the AWS Config backend into the Resource Groups Tagging
// API. Its ARN service namespace is "config", not "awsconfig" (confirmed against
// services/awsconfig's recorderArn/connectorArn helpers, which hand-build
// "arn:aws:config:..." rather than using pkgs/arn). Configuration recorders and
// connectors are each their own flat "type/id" ARN, so resourceTypeFromARN derives the
// per-resource type.
//
//nolint:dupl // structurally mirrors wireTaggingComprehend (both need mapToTagSlice) but wires an unrelated backend
func wireTaggingAWSConfig(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*awsconfigbackend.Handler)
	if !ok {
		return
	}

	acfgBk := h.Backend
	if acfgBk == nil {
		return
	}

	wireTaggingARNResources(
		bk, "config",
		func(arn string) string { return resourceTypeFromARN(arn, "config") },
		func() []taggedARNEntry {
			items := acfgBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		func(arn string, tags map[string]string) error {
			return acfgBk.TagResource(arn, mapToTagSlice(tags, func(k, v string) awsconfigbackend.Tag {
				return awsconfigbackend.Tag{Key: k, Value: v}
			}))
		},
		acfgBk.UntagResource,
	)
}

// wireTaggingScheduler wires the EventBridge Scheduler backend into the Resource
// Groups Tagging API. Schedules and schedule groups are each their own flat "type/..."
// ARN -- a schedule's ARN is "schedule/{group}/{name}" (3 segments, still a single
// kind since the group name is a namespacing prefix, not a second nested resource
// kind), so resourceTypeFromARN derives the per-resource type.
// TagResource/UntagResource take a context (used for region resolution), so this uses
// wireTaggingCtxARNResources.
func wireTaggingScheduler(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*schedulerbackend.Handler)
	if !ok {
		return
	}

	schBk, ok := h.Backend.(*schedulerbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingCtxARNResources(
		bk, "scheduler",
		func(arn string) string { return resourceTypeFromARN(arn, "scheduler") },
		func() []taggedARNEntry {
			items := schBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		schBk.TagResource,
		schBk.UntagResource,
	)
}

// wireTaggingCE wires the Cost Explorer backend into the Resource Groups Tagging API.
// CE keeps tags for cost categories, anomaly monitors, and anomaly subscriptions in
// three separate stores, each with a flat "type/id" ARN (e.g. "costcategory/{name}",
// "anomalymonitor/{id}", "anomalysubscription/{id}"), so resourceTypeFromARN derives
// the per-resource type. The provider's registered name is "Ce" (see
// services/ce/provider.go), but its ARNs use the real AWS "ce" service namespace.
func wireTaggingCE(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*cebackend.Handler)
	if !ok {
		return
	}

	ceBk := h.Backend
	if ceBk == nil {
		return
	}

	wireTaggingARNResources(
		bk, "ce",
		func(arn string) string { return resourceTypeFromARN(arn, "ce") },
		func() []taggedARNEntry {
			items := ceBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		ceBk.TagResource,
		ceBk.UntagResource,
	)
}

// wireTaggingMediaPackage wires the MediaPackage backend into the Resource Groups
// Tagging API. MediaPackage keeps tags for channels and origin endpoints
// ("channels/{id}", "origin_endpoints/{id}", see resourceTypeChannel/
// resourceTypeOriginEndpoint in services/mediapackage/store.go) in one flat ARN-keyed
// map, so resourceTypeFromARN derives the per-resource type.
func wireTaggingMediaPackage(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*mediapackagebackend.Handler)
	if !ok {
		return
	}

	mpBk, ok := h.Backend.(*mediapackagebackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "mediapackage",
		func(arn string) string { return resourceTypeFromARN(arn, "mediapackage") },
		func() []taggedARNEntry {
			items := mpBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		mpBk.TagResource,
		mpBk.UntagResource,
	)
}

// wireTaggingSWF wires the SWF backend into the Resource Groups Tagging API. SWF tags
// only domains, whose ARN resource segment is "/domain/{name}" with a leading slash
// (see swfARNRegex) -- resourceTypeFromARN would treat that leading slash as an empty
// type, so this uses a constant resource type instead.
func wireTaggingSWF(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*swfbackend.Handler)
	if !ok {
		return
	}

	swfBk, ok := h.Backend.(*swfbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "swf", constantResourceType("swf:domain"),
		func() []taggedARNEntry {
			items := swfBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		swfBk.TagResource,
		swfBk.UntagResource,
	)
}

// wireTaggingFIS wires the FIS backend into the Resource Groups Tagging API. FIS tags
// the account's safety lever, experiment templates, and experiments, each with a flat
// "type/id" ARN ("safety-lever", "experiment-template/{id}", "experiment/{id}"), so
// resourceTypeFromARN derives the per-resource type.
func wireTaggingFIS(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*fisbackend.Handler)
	if !ok {
		return
	}

	fisBk, ok := h.Backend.(*fisbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "fis",
		func(arn string) string { return resourceTypeFromARN(arn, "fis") },
		func() []taggedARNEntry {
			items := fisBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		fisBk.TagResource,
		fisBk.UntagResource,
	)
}

// wireTaggingCodeConnections wires the CodeConnections backend into the Resource
// Groups Tagging API. CodeConnections tags connections, hosts, and repository links,
// each with a flat "type/..." ARN ("connection/{id}", "host/{name}/{id8}",
// "repository-link/{id}"), so resourceTypeFromARN derives the per-resource type from
// the first segment (the extra segments on host ARNs are part of the identifier, not
// a distinct nested kind).
func wireTaggingCodeConnections(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*codeconnectionsbackend.Handler)
	if !ok {
		return
	}

	ccBk := h.Backend
	if ccBk == nil {
		return
	}

	wireTaggingCtxARNResources(
		bk, "codeconnections",
		func(arn string) string { return resourceTypeFromARN(arn, "codeconnections") },
		func() []taggedARNEntry {
			items := ccBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		ccBk.TagResource,
		ccBk.UntagResource,
	)
}

// wireTaggingMediaStore wires the MediaStore backend into the Resource Groups Tagging
// API. MediaStore tags only containers ("container/{name}", see containerARN), a
// single resource kind, so this uses a constant resource type.
func wireTaggingMediaStore(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*mediastorebackend.Handler)
	if !ok {
		return
	}

	msBk, ok := h.Backend.(*mediastorebackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingCtxARNResources(
		bk, "mediastore", constantResourceType("mediastore:container"),
		func() []taggedARNEntry {
			items := msBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		msBk.TagResource,
		msBk.UntagResource,
	)
}

// wireTaggingMWAA wires the MWAA backend into the Resource Groups Tagging API. MWAA
// tags only environments ("environment/{name}", see envARN callers), a single resource
// kind, so this uses a constant resource type. MWAA's ARNs use the real AWS "airflow"
// service namespace, not "mwaa" (see the arn.Build calls in services/mwaa).
func wireTaggingMWAA(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*mwaabackend.Handler)
	if !ok {
		return
	}

	mwaaBk, ok := h.Backend.(*mwaabackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingCtxARNResources(
		bk, "airflow", constantResourceType("airflow:environment"),
		func() []taggedARNEntry {
			items := mwaaBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		mwaaBk.TagResource,
		mwaaBk.UntagResource,
	)
}

// wireTaggingPipes wires the Pipes backend into the Resource Groups Tagging API. Pipes
// tags only pipes ("pipe/{name}", see pipeARN), a single resource kind, so this uses a
// constant resource type.
func wireTaggingPipes(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*pipesbackend.Handler)
	if !ok {
		return
	}

	pipesBk := h.Backend
	if pipesBk == nil {
		return
	}

	wireTaggingCtxARNResources(
		bk, "pipes", constantResourceType("pipes:pipe"),
		func() []taggedARNEntry {
			items := pipesBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		pipesBk.TagResource,
		pipesBk.UntagResource,
	)
}

// wireTaggingMacie2 wires the Macie2 backend into the Resource Groups Tagging API.
// Macie2 tags allow lists, custom data identifiers, findings filters, and
// classification jobs (all flat "type/id" ARNs, see InMemoryBackend.isKnownARN and
// each resource's own arn.Build call site), so resourceTypeFromARN derives the
// per-resource type.
func wireTaggingMacie2(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*macie2backend.Handler)
	if !ok {
		return
	}

	mBk, ok := h.Backend.(*macie2backend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "macie2",
		func(arnStr string) string { return resourceTypeFromARN(arnStr, "macie2") },
		func() []taggedARNEntry {
			items := mBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		mBk.TagResource,
		mBk.UntagResource,
	)
}

// wireTaggingManagedBlockchain wires the Managed Blockchain backend into the Resource
// Groups Tagging API. It tags networks, members, nodes, and accessors (all flat
// "type/id" ARNs, e.g. "networks/{id}", see each resource's own arn.Build call site --
// note the plural resource-type segments), so resourceTypeFromARN derives the
// per-resource type. Invitations carry no Tags field and are never returned.
func wireTaggingManagedBlockchain(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*managedblockchainbackend.Handler)
	if !ok {
		return
	}

	mbBk, ok := h.Backend.(*managedblockchainbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "managedblockchain",
		func(arnStr string) string { return resourceTypeFromARN(arnStr, "managedblockchain") },
		func() []taggedARNEntry {
			items := mbBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		mbBk.TagResource,
		mbBk.UntagResource,
	)
}

// wireTaggingMediaConvert wires the MediaConvert backend into the Resource Groups
// Tagging API. It tags job templates, jobs, presets, and queues (all flat "type/id"
// ARNs, e.g. "jobTemplates/{name}", see each resource's own arn.Build call site), so
// resourceTypeFromARN derives the per-resource type. MediaConvert's own
// TagResource/UntagResource return no error, unlike wireTaggingARNResources' tagFn/
// untagFn shape, so they are wrapped below.
func wireTaggingMediaConvert(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*mediaconvertbackend.Handler)
	if !ok {
		return
	}

	mcBk, ok := h.Backend.(*mediaconvertbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "mediaconvert",
		func(arnStr string) string { return resourceTypeFromARN(arnStr, "mediaconvert") },
		func() []taggedARNEntry {
			items := mcBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		func(arnStr string, newTags map[string]string) error {
			mcBk.TagResource(arnStr, newTags)

			return nil
		},
		func(arnStr string, keys []string) error {
			mcBk.UntagResource(arnStr, keys)

			return nil
		},
	)
}

// wireTaggingDataSync wires the DataSync backend into the Resource Groups Tagging API.
// It tags agents, locations, and tasks (all flat "type/id" ARNs, see store.go's
// agentARN/locationARN/taskARN), so resourceTypeFromARN derives the per-resource type.
// Task executions build a nested "task/{id}/execution/{id}" ARN but isKnownResource
// never recognizes execution ARNs, so they can never be tagged and never appear here.
func wireTaggingDataSync(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*datasyncbackend.Handler)
	if !ok {
		return
	}

	dsBk, ok := h.Backend.(*datasyncbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "datasync",
		func(arnStr string) string { return resourceTypeFromARN(arnStr, "datasync") },
		func() []taggedARNEntry {
			items := dsBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		dsBk.TagResource,
		dsBk.UntagResource,
	)
}

// wireTaggingCodeDeploy wires the CodeDeploy backend into the Resource Groups Tagging
// API. It tags applications and deployment groups, whose ARNs use a colon (not slash)
// before the resource segment ("application:{name}", "deploymentgroup:{app}/{group}",
// see ApplicationARN/DeploymentGroupARN) -- resourceTypeFromARN handles both "/" and
// ":" separators, so it still derives the per-resource type correctly.
func wireTaggingCodeDeploy(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*codedeploybackend.Handler)
	if !ok {
		return
	}

	cdBk := h.Backend
	if cdBk == nil {
		return
	}

	wireTaggingARNResources(
		bk, "codedeploy",
		func(arnStr string) string { return resourceTypeFromARN(arnStr, "codedeploy") },
		func() []taggedARNEntry {
			items := cdBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		cdBk.TagResource,
		cdBk.UntagResource,
	)
}

// wireTaggingInspector2 wires the Inspector2 backend into the Resource Groups Tagging
// API. Findings filters are the only resource kind reachable through TaggedResources
// (resourceExists only recognizes filter ARNs or an ARN already present in the tag
// store, and only CreateFilter seeds the tag store at creation time), giving a flat
// "filter/{id}" ARN, so resourceTypeFromARN derives the per-resource type.
func wireTaggingInspector2(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*inspector2backend.Handler)
	if !ok {
		return
	}

	iBk, ok := h.Backend.(*inspector2backend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "inspector2",
		func(arnStr string) string { return resourceTypeFromARN(arnStr, "inspector2") },
		func() []taggedARNEntry {
			items := iBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		iBk.TagResource,
		iBk.UntagResource,
	)
}

// wireTaggingRAM wires the RAM backend into the Resource Groups Tagging API.
// Real RAM's TagResource only tags resource shares (confirmed against
// resourceShares.Get in ram/tags.go -- permission and invitation ARNs, also
// built via arn.Build in ram/permissions.go and ram/share_invitations.go, are
// never checked), so this uses a constant resource type rather than
// resourceTypeFromARN.
func wireTaggingRAM(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*rambackend.Handler)
	if !ok {
		return
	}

	rBk, ok := h.Backend.(*rambackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "ram", constantResourceType("ram:resource-share"),
		func() []taggedARNEntry {
			items := rBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		rBk.TagResource,
		rBk.UntagResource,
	)
}

// wireTaggingRekognition wires the Rekognition backend into the Resource
// Groups Tagging API. It tags collections, stream processors, and project
// versions (all flat "type/id" ARNs, see rekognition/tags.go's resourceExists
// doc comment), so resourceTypeFromARN derives the per-resource type. Bare
// project ARNs (arn.Build in rekognition/projects.go) are never accepted --
// only a project *version* ARN is taggable, matching the real API.
func wireTaggingRekognition(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*rekognitionbackend.Handler)
	if !ok {
		return
	}

	rBk, ok := h.Backend.(*rekognitionbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "rekognition",
		func(arnStr string) string { return resourceTypeFromARN(arnStr, "rekognition") },
		func() []taggedARNEntry {
			items := rBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		rBk.TagResource,
		rBk.UntagResource,
	)
}

// wireTaggingTranslate wires the Translate backend into the Resource Groups
// Tagging API. It tags terminologies and parallel data (flat "type/id" ARNs,
// see translate/terminologies.go and translate/parallel_data.go's arn.Build
// call sites), so resourceTypeFromARN derives the per-resource type.
func wireTaggingTranslate(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*translatebackend.Handler)
	if !ok {
		return
	}

	tBk := h.Backend
	if tBk == nil {
		return
	}

	wireTaggingARNResources(
		bk, "translate",
		func(arnStr string) string { return resourceTypeFromARN(arnStr, "translate") },
		func() []taggedARNEntry {
			items := tBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		tBk.TagResource,
		tBk.UntagResource,
	)
}

// wireTaggingAppStream wires the AppStream backend into the Resource Groups
// Tagging API. It tags every flat "type/id" ARN kind AppStream seeds into its
// tag store at creation time (stacks, app blocks, fleets, applications,
// images -- see appstream/tags.go's isKnownARN), so resourceTypeFromARN
// derives the per-resource type. Directory configs and users build ARNs too
// (appstream/directory_configs.go, appstream/users.go) but are never seeded,
// so they can never be tagged and never appear here.
func wireTaggingAppStream(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*appstreambackend.Handler)
	if !ok {
		return
	}

	aBk, ok := h.Backend.(*appstreambackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "appstream",
		func(arnStr string) string { return resourceTypeFromARN(arnStr, "appstream") },
		func() []taggedARNEntry {
			items := aBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		aBk.TagResource,
		aBk.UntagResource,
	)
}

// wireTaggingMediaTailor wires the MediaTailor backend into the Resource
// Groups Tagging API. It tags playback configurations, channels, source
// locations, and functions (all flat "type/id" ARNs, see
// mediatailor/store.go's arn.Build call sites), so resourceTypeFromARN
// derives the per-resource type.
func wireTaggingMediaTailor(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*mediatailorbackend.Handler)
	if !ok {
		return
	}

	mBk, ok := h.Backend.(*mediatailorbackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "mediatailor",
		func(arnStr string) string { return resourceTypeFromARN(arnStr, "mediatailor") },
		func() []taggedARNEntry {
			items := mBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		mBk.TagResource,
		mBk.UntagResource,
	)
}

// wireTaggingVPCLattice wires the VPC Lattice backend into the Resource
// Groups Tagging API. Its own ARN namespace is "vpc-lattice", not
// "vpclattice" (see vpclattice/store.go's arnService constant) -- the same
// class of trap as MWAA's "airflow" namespace. It tags many flat "type/id"
// resource kinds (service networks, services, target groups, listeners,
// rules, resource gateways/configurations, domain verifications,
// associations), so resourceTypeFromARN derives the per-resource type.
func wireTaggingVPCLattice(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*vpclatticebackend.Handler)
	if !ok {
		return
	}

	vBk, ok := h.Backend.(*vpclatticebackend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingARNResources(
		bk, "vpc-lattice",
		func(arnStr string) string { return resourceTypeFromARN(arnStr, "vpc-lattice") },
		func() []taggedARNEntry {
			items := vBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		vBk.TagResource,
		vBk.UntagResource,
	)
}

// codepipelineResourceType derives the resource-type string for a CodePipeline
// ARN. Its flat tag store mixes two ARN shapes under one namespace: pipeline
// ARNs are a bare name with no "/" or ":" separator (arn.Build in
// codepipeline/store.go's buildPipelineARN), so resourceTypeFromARN's fallback
// returns the service alone ("codepipeline"); webhook ARNs carry a
// "webhook:{name}" resource segment (buildWebhookARN), which
// resourceTypeFromARN already parses correctly into "codepipeline:webhook".
// This turns the pipeline fallback into an explicit "codepipeline:pipeline" so
// the two kinds are never conflated under one bare type string.
func codepipelineResourceType(resourceARN string) string {
	t := resourceTypeFromARN(resourceARN, "codepipeline")
	if t == "codepipeline" {
		return "codepipeline:pipeline"
	}

	return t
}

// wireTaggingCodePipeline wires the CodePipeline backend into the Resource
// Groups Tagging API. TagResource/UntagResource take a context.Context
// (region is resolved from it, see codepipeline/store.go's getRegion) and a
// []Tag rather than a bare map, so the tagger closure below adapts the shape.
func wireTaggingCodePipeline(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*codepipelinebackend.Handler)
	if !ok {
		return
	}

	cpBk := h.Backend
	if cpBk == nil {
		return
	}

	wireTaggingCtxARNResources(
		bk, "codepipeline", codepipelineResourceType,
		func() []taggedARNEntry {
			items := cpBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		func(ctx context.Context, arnStr string, newTags map[string]string) error {
			tagList := make([]codepipelinebackend.Tag, 0, len(newTags))
			for k, v := range newTags {
				tagList = append(tagList, codepipelinebackend.Tag{Key: k, Value: v})
			}

			return cpBk.TagResource(ctx, arnStr, tagList)
		},
		cpBk.UntagResource,
	)
}

// wireTaggingKinesisAnalyticsV2 wires the Kinesis Data Analytics v2 backend
// into the Resource Groups Tagging API. Its ARN namespace is "kinesisanalytics"
// (see kinesisanalyticsv2/tags.go's findByARN callers and the shared
// arn.Build("kinesisanalytics", ...) call site), not "kinesisanalyticsv2" --
// the same trap class as MWAA/VPC-Lattice. This namespace is also used by the
// separate, still-unwired kinesisanalytics (v1) service; wiring both would
// need the same registration-order ownership check as wireTaggingDocDB/
// wireTaggingNeptune use for their shared "rds" namespace. Only application
// ARNs exist ("application/{name}"), so this uses a constant resource type.
func wireTaggingKinesisAnalyticsV2(bk resourcegroupstaggingapibackend.StorageBackend, reg service.Registerable) {
	h, ok := reg.(*kinesisanalyticsv2backend.Handler)
	if !ok {
		return
	}

	kaBk, ok := h.Backend.(*kinesisanalyticsv2backend.InMemoryBackend)
	if !ok {
		return
	}

	wireTaggingCtxARNResources(
		bk, "kinesisanalytics", constantResourceType("kinesisanalytics:application"),
		func() []taggedARNEntry {
			items := kaBk.TaggedResources()
			out := make([]taggedARNEntry, 0, len(items))
			for _, item := range items {
				out = append(out, taggedARNEntry{ARN: item.ARN, Tags: item.Tags})
			}

			return out
		},
		func(ctx context.Context, arnStr string, newTags map[string]string) error {
			tagList := make([]kinesisanalyticsv2backend.Tag, 0, len(newTags))
			for k, v := range newTags {
				tagList = append(tagList, kinesisanalyticsv2backend.Tag{Key: k, Value: v})
			}

			return kaBk.TagResource(ctx, arnStr, tagList)
		},
		kaBk.UntagResource,
	)
}

// startPprofServer starts an opt-in pprof HTTP server for local profiling and
// Profile-Guided Optimization (PGO) data collection. It is OFF by default and
// only starts when GOPHERSTACK_PPROF_ADDR is set to a non-empty address
// (e.g. "localhost:6060"). The pprof handlers are served on a dedicated
// *http.Server bound to a private *http.ServeMux, deliberately separate from
// the main application echo server on cli.Port: pprof endpoints expose
// process internals (stack traces, heap dumps, CPU profiles) that must never
// be reachable on the primary port, and must never be registered on
// http.DefaultServeMux (which could leak them into any other package that
// mounts it). This is intended for local development / profiling only and
// should not be enabled in shared or production-like environments.
func startPprofServer(log *slog.Logger) {
	addr := os.Getenv("GOPHERSTACK_PPROF_ADDR")
	if addr == "" {
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", nhpprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", nhpprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", nhpprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", nhpprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", nhpprof.Trace)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: defaultReadHeaderTimeout,
	}

	log.Info("pprof profiling enabled", "addr", addr)

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Warn("pprof server stopped", "error", err)
		}
	}()
}

func startServer(ctx context.Context, port string, e *echo.Echo, tlsCfg tlsSettings) error {
	log := logger.Load(ctx)

	if port[0] != ':' {
		port = ":" + port
	}

	scheme := "http"
	if tlsCfg.enabled {
		scheme = "https"
	}

	log.InfoContext(ctx, "Starting Gopherstack (DynamoDB + S3)", "port", port, "scheme", scheme)
	log.InfoContext(ctx, "  DynamoDB endpoint", "url", scheme+"://localhost"+port)
	log.InfoContext(ctx, "  S3 endpoint      ", "url", scheme+"://localhost"+port+" (path-style)")
	log.InfoContext(ctx, "  Dashboard        ", "url", scheme+"://localhost"+port+"/dashboard")

	server := &http.Server{
		Addr:    port,
		Handler: e,
		// Protocols set below; under TLS we omit the unencrypted-h2 setting so
		// the standard h2 ALPN negotiation applies.
		ReadTimeout:       defaultTimeout,
		ReadHeaderTimeout: defaultReadHeaderTimeout, // Security best practice
		// WriteTimeout intentionally 0: long-lived ConnectRPC streams
		// (StreamConsole, StreamMetrics) must outlive the per-request budget.
		// The default 30s WriteTimeout cut the metrics stream mid-frame after
		// ~30s, surfacing as ERR_INCOMPLETE_CHUNK in the browser. Slow-write
		// attacks are mitigated by ReadHeaderTimeout above; for an in-memory
		// dev simulator this is an acceptable trade-off.
		WriteTimeout: 0,
		IdleTimeout:  defaultTimeout,
	}

	if !tlsCfg.enabled {
		protocols := new(http.Protocols)
		protocols.SetHTTP1(true)
		protocols.SetUnencryptedHTTP2(true)
		server.Protocols = protocols
	}

	errChan := make(chan error, 1)
	go func() {
		if err := serveHTTP(server, tlsCfg); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errChan <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.InfoContext(ctx, "Shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			log.ErrorContext(ctx, "Server shutdown failed", "error", err)

			return err
		}

		return nil
	case err := <-errChan:
		log.ErrorContext(ctx, "Failed to start server", "error", err)

		return err
	}
}

// serveHTTP starts the server, choosing HTTP, file-based TLS, or self-signed TLS
// based on tlsCfg. It blocks until the server stops.
func serveHTTP(server *http.Server, tlsCfg tlsSettings) error {
	if !tlsCfg.enabled {
		return server.ListenAndServe()
	}

	if tlsCfg.certFile != "" && tlsCfg.keyFile != "" {
		return server.ListenAndServeTLS(tlsCfg.certFile, tlsCfg.keyFile)
	}

	// No cert supplied: generate a self-signed certificate in memory.
	cert, err := generateSelfSignedCert()
	if err != nil {
		return fmt.Errorf("generate self-signed certificate: %w", err)
	}

	server.TLSConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	// Empty cert/key paths => server uses TLSConfig.Certificates.
	return server.ListenAndServeTLS("", "")
}

// generateSelfSignedCert creates an in-memory self-signed certificate valid for
// localhost / 127.0.0.1 / ::1, suitable for an opt-in dev HTTPS listener.
func generateSelfSignedCert() (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate key: %w", err)
	}

	serialLimit := new(big.Int).Lsh(big.NewInt(1), selfSignedSerialBits)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate serial: %w", err)
	}

	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "gopherstack", Organization: []string{"gopherstack"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(selfSignedValidity),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{localhostName},
		IPAddresses:           []net.IP{net.IPv4(loopbackIPv4Octet, 0, 0, 1), net.IPv6loopback},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create certificate: %w", err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("marshal key: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	return tls.X509KeyPair(certPEM, keyPEM)
}

// buildLogger converts the CLI log-level string to a [slog.Logger].
func buildLogger(level string) *slog.Logger {
	var slogLevel slog.Level

	switch strings.ToLower(strings.TrimSpace(level)) {
	case logLevelDebug:
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	return logger.NewLogger(slogLevel)
}

// healthResponse is the JSON body returned by the health endpoint.
type healthResponse struct {
	// Status is always "ok" when the server is running.
	Status string `json:"status"`
	// Version is the build-time version of Gopherstack.
	Version string `json:"version"`
	// Services lists all registered mock AWS services.
	Services []string `json:"services"`
	// Goroutines is the current number of live goroutines (runtime.NumGoroutine).
	// A sustained upward trend indicates a goroutine leak.
	Goroutines int `json:"goroutines"`
	// HeapAllocB is the number of heap bytes currently allocated (runtime.MemStats.HeapAlloc).
	HeapAllocB uint64 `json:"heap_alloc_bytes"`
	// HeapInuseB is the number of heap bytes in use (runtime.MemStats.HeapInuse).
	HeapInuseB uint64 `json:"heap_inuse_bytes"`
	// NumGC is the total number of completed GC cycles (runtime.MemStats.NumGC).
	NumGC uint32 `json:"num_gc"`
}

// localstackHealthResponse is the JSON body returned by LocalStack-compatible health endpoints.
type localstackHealthResponse struct {
	Services map[string]string `json:"services"`
	Version  string            `json:"version"`
	Edition  string            `json:"edition"`
}

// localstackInitResponse is the JSON body returned by LocalStack-compatible init endpoints.
type localstackInitResponse struct {
	Scripts   []string `json:"scripts"`
	Completed bool     `json:"completed"`
}

func buildLocalstackHealthHandler(services []service.Registerable) echo.HandlerFunc {
	return func(c *echo.Context) error {
		svcMap := make(map[string]string, len(services))
		for _, svc := range services {
			svcMap[strings.ToLower(svc.Name())] = "available"
		}

		return c.JSON(http.StatusOK, localstackHealthResponse{
			Services: svcMap,
			Version:  version.Get(),
			Edition:  "community",
		})
	}
}

func buildLocalstackInitHandler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return c.JSON(http.StatusOK, localstackInitResponse{
			Completed: true,
			Scripts:   []string{},
		})
	}
}

type localstackInfoResponse struct {
	Version   string `json:"version"`
	Edition   string `json:"edition"`
	SessionID string `json:"session_id"`
	IsAuth    bool   `json:"is_auth"`
}

func buildLocalstackInfoHandler() echo.HandlerFunc {
	return func(c *echo.Context) error {
		return c.JSON(http.StatusOK, localstackInfoResponse{
			Version:   version.Get(),
			Edition:   "community",
			IsAuth:    false,
			SessionID: "00000000-0000-0000-0000-000000000000",
		})
	}
}

func setupChaosAndRegistry(
	e *echo.Echo,
	log *slog.Logger,
	cli *CLI,
	services []service.Registerable,
) error {
	faultStore := cli.faultStore
	chaosGroup := e.Group("/_gopherstack/chaos")
	wireFISFaultStore(cli.fisHandler, faultStore)

	registry, err := setupRegistry(
		e,
		log,
		services,
		cli.LatencyMs,
		cli.EnforceIAM,
		cli.GetGlobalConfig(),
		faultStore,
	)
	if err != nil {
		return err
	}

	chaos.RegisterRoutes(chaosGroup, faultStore, registry)

	return nil
}

func setupRegistry(
	e *echo.Echo,
	log *slog.Logger,
	services []service.Registerable,
	latencyMs int,
	enforceIAM bool,
	globalCfg *config.GlobalConfig,
	faultStore *chaos.FaultStore,
) (*service.Registry, error) {
	registry := service.NewRegistry()

	if latencyMs > 0 {
		registry.SetLatencyMs(latencyMs)
	}

	// Wire the live CloudTrail backend (if registered) as the registry's global
	// management-event recorder. This makes every mutating call to every
	// registered service show up in CloudTrail LookupEvents, without any
	// per-service integration code.
	if ctRecorder := findCloudTrailRecorder(services); ctRecorder != nil {
		registry.SetCloudTrailRecorder(ctRecorder)
	}

	// Chaos middleware runs outside the telemetry wrapper (as a global middleware).
	// It extracts service/region/operation directly from the HTTP request headers so
	// it does not depend on context values that are only set by the telemetry wrapper.
	registry.Use(chaos.Middleware(faultStore))

	// Records the region of every request so the dashboard can discover which
	// regions hold data (see /dashboard/api/system/regions) without fanning
	// out to every AWS region.
	registry.Use(service.RegionTrackingMiddleware(globalCfg.GetRegion()))

	principalResolvers := buildPrincipalResolvers(services)
	if len(principalResolvers) > 0 {
		registry.Use(service.Middleware(principalMiddleware(principalResolvers)))
	}

	if enforceIAM {
		iamBackend := findIAMBackend(services)
		if iamBackend != nil {
			log.Info("IAM policy enforcement enabled")

			ecfg := iambackend.EnforcementConfig{
				Global:            globalCfg,
				ResourceProviders: buildResourcePolicyProviders(services),
				ActionExtractors:  buildActionExtractors(services),
			}

			registry.Use(service.Middleware(iambackend.EnforcementMiddleware(iamBackend, ecfg)))
		} else {
			log.Warn("IAM enforcement requested but IAM backend not found; enforcement disabled")
		}
	}

	for _, svc := range services {
		if err := registry.Register(svc); err != nil {
			log.Error("Failed to register service", "service", svc.Name(), "error", err)

			return nil, err
		}
	}

	router := service.NewServiceRouter(registry)
	e.Use(router.RouteHandler())

	return registry, nil
}

// findCloudTrailRecorder locates the live CloudTrail backend from the service
// list, so the registry can wire it as its global management-event recorder
// instead of constructing a second, disconnected CloudTrail backend.
func findCloudTrailRecorder(services []service.Registerable) service.CloudTrailRecorder {
	for _, svc := range services {
		if rec, ok := svc.(service.CloudTrailRecorder); ok {
			return rec
		}
	}

	return nil
}

// findIAMBackend locates the IAM EnforcementBackend from the service list.
func findIAMBackend(services []service.Registerable) iambackend.EnforcementBackend {
	for _, svc := range services {
		if h, ok := svc.(*iambackend.Handler); ok {
			if b, ok2 := h.Backend.(iambackend.EnforcementBackend); ok2 {
				return b
			}
		}
	}

	return nil
}

// buildActionExtractors collects ActionExtractor implementations from all registered
// services. Services that implement the iam.ActionExtractor interface are automatically
// included so their REST-API action mappings are used by the enforcement middleware.
func buildActionExtractors(services []service.Registerable) []iambackend.ActionExtractor {
	extractors := make([]iambackend.ActionExtractor, 0, len(services))

	for _, svc := range services {
		if ae, ok := svc.(iambackend.ActionExtractor); ok {
			extractors = append(extractors, ae)
		} else {
			extractors = append(extractors, iambackend.NewRegisterableActionExtractor(svc))
		}
	}

	return extractors
}

// extractStorageResourcePolicyProvider extracts a ResourcePolicyProvider adapter for storage/compute services.
func extractStorageResourcePolicyProvider(svc service.Registerable) iambackend.ResourcePolicyProvider {
	switch h := svc.(type) {
	case *s3backend.S3Handler:
		if b, ok := h.Backend.(s3PolicyBackend); ok {
			return &s3PolicyAdapter{backend: b}
		}
	case *sqsbackend.Handler:
		if b, ok := h.Backend.(sqsPolicyBackend); ok {
			return &sqsPolicyAdapter{backend: b}
		}
	case *kmsbackend.Handler:
		if b, ok := h.Backend.(kmsPolicyBackend); ok {
			return &kmsPolicyAdapter{backend: b}
		}
	case *secretsmanagerbackend.Handler:
		if b, ok := h.Backend.(secretsManagerPolicyBackend); ok {
			return &secretsManagerPolicyAdapter{backend: b}
		}
	case *lambdabackend.Handler:
		if b, ok := h.Backend.(lambdaPolicyBackend); ok {
			return &lambdaPolicyAdapter{backend: b}
		}
	}

	return nil
}

// extractExtendedResourcePolicyProvider extracts a ResourcePolicyProvider adapter for extended services.
func extractExtendedResourcePolicyProvider(svc service.Registerable) iambackend.ResourcePolicyProvider {
	switch h := svc.(type) {
	case *ecrbackend.Handler:
		if b, ok := h.Backend.(ecrPolicyBackend); ok {
			return &ecrPolicyAdapter{backend: b}
		}
	case *snsbackend.Handler:
		if b, ok := h.Backend.(snsPolicyBackend); ok {
			return &snsPolicyAdapter{backend: b}
		}
	case *ebbackend.Handler:
		if b, ok := h.Backend.(ebPolicyBackend); ok {
			return &ebPolicyAdapter{backend: b}
		}
	case *bedrockbackend.Handler:
		if h.Backend != nil {
			return &bedrockPolicyAdapter{backend: h.Backend}
		}
	}

	return nil
}

// extractResourcePolicyProvider extracts a ResourcePolicyProvider adapter for a service if supported.
func extractResourcePolicyProvider(svc service.Registerable) iambackend.ResourcePolicyProvider {
	if p := extractStorageResourcePolicyProvider(svc); p != nil {
		return p
	}

	return extractExtendedResourcePolicyProvider(svc)
}

// buildResourcePolicyProviders builds a list of ResourcePolicyProvider adapters
// from the registered service backends that support resource-based policies.
func buildResourcePolicyProviders(
	services []service.Registerable,
) []iambackend.ResourcePolicyProvider {
	var providers []iambackend.ResourcePolicyProvider

	for _, svc := range services {
		if p := extractResourcePolicyProvider(svc); p != nil {
			providers = append(providers, p)
		}
	}

	return providers
}

// buildPrincipalResolvers collects PrincipalResolver implementations from registered service backends.
func buildPrincipalResolvers(services []service.Registerable) awsmeta.PrincipalResolverChain {
	var chain awsmeta.PrincipalResolverChain

	for _, svc := range services {
		switch h := svc.(type) {
		case *iambackend.Handler:
			if r, ok := h.Backend.(awsmeta.PrincipalResolver); ok {
				chain = append(chain, r)
			}
		case *stsbackend.Handler:
			if r, ok := h.Backend.(awsmeta.PrincipalResolver); ok {
				chain = append(chain, r)
			}
		}
	}

	return chain
}

// principalMiddleware populates the resolved caller Principal onto the request context.
func principalMiddleware(resolvers awsmeta.PrincipalResolverChain) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			req := c.Request()
			ctx := req.Context()
			meta := awsmeta.Get(ctx)

			if meta != nil && meta.AccessKeyID != "" && meta.Principal == nil {
				if p, ok := resolvers.ResolvePrincipal(ctx, meta.AccessKeyID, meta.SecurityToken); ok && p != nil {
					meta.Principal = p
					ctx = awsmeta.Set(ctx, meta)
					c.SetRequest(req.WithContext(ctx))
				}
			}

			return next(c)
		}
	}
}

// s3PolicyBackend is the minimal S3 backend interface needed for bucket policies.
type s3PolicyBackend interface {
	GetBucketPolicy(ctx context.Context, bucketName string) (string, error)
}

// sqsPolicyBackend is the minimal SQS backend interface needed for queue policies.
type sqsPolicyBackend interface {
	GetQueueAttributes(
		input *sqsbackend.GetQueueAttributesInput,
	) (*sqsbackend.GetQueueAttributesOutput, error)
}

// kmsPolicyBackend is the minimal KMS backend interface needed for key policies.
type kmsPolicyBackend interface {
	GetKeyPolicy(ctx context.Context, input *kmsbackend.GetKeyPolicyInput) (*kmsbackend.GetKeyPolicyOutput, error)
}

// secretsManagerPolicyBackend is the minimal Secrets Manager backend interface needed for resource policies.
type secretsManagerPolicyBackend interface {
	GetResourcePolicy(
		ctx context.Context,
		input *secretsmanagerbackend.GetResourcePolicyInput,
	) (*secretsmanagerbackend.GetResourcePolicyOutput, error)
}

// lambdaPolicyBackend is the minimal Lambda backend interface needed for function policies.
type lambdaPolicyBackend interface {
	GetPolicy(functionName, qualifier string) (*lambdabackend.GetPolicyOutput, error)
}

// ecrPolicyBackend is the minimal ECR backend interface needed for repository policies.
type ecrPolicyBackend interface {
	GetRepositoryPolicy(ctx context.Context, repositoryName string) (*ecrbackend.RepositoryPolicyResult, error)
}

// snsPolicyBackend is the minimal SNS backend interface needed for topic policies.
type snsPolicyBackend interface {
	GetTopicAttributes(topicArn string) (map[string]string, error)
}

// ebPolicyBackend is the minimal EventBridge backend interface needed for bus policies.
type ebPolicyBackend interface {
	DescribeEventBus(ctx context.Context, name string) (*ebbackend.EventBus, error)
}

// bedrockPolicyBackend is the minimal Bedrock backend interface needed for resource policies.
type bedrockPolicyBackend interface {
	GetResourcePolicy(resourceArn string) (*bedrockbackend.ResourcePolicy, error)
}

// s3PolicyAdapter wraps an S3 backend to implement ResourcePolicyProvider.
// It handles ARNs of the form arn:aws:s3:::bucket or arn:aws:s3:::bucket/key.
type s3PolicyAdapter struct {
	backend s3PolicyBackend
}

func (a *s3PolicyAdapter) GetResourcePolicy(
	ctx context.Context,
	resourceARN string,
) (string, error) {
	const prefix = "arn:aws:s3:::"
	if !strings.HasPrefix(resourceARN, prefix) {
		return "", nil
	}

	path := strings.TrimPrefix(resourceARN, prefix)
	bucketName, _, _ := strings.Cut(path, "/")

	if bucketName == "" {
		return "", nil
	}

	return a.backend.GetBucketPolicy(ctx, bucketName)
}

// sqsPolicyAdapter wraps a SQS backend to implement ResourcePolicyProvider.
// It handles ARNs of the form arn:aws:sqs:region:account:queue-name.
type sqsPolicyAdapter struct {
	backend sqsPolicyBackend
}

func (a *sqsPolicyAdapter) GetResourcePolicy(
	_ context.Context,
	resourceARN string,
) (string, error) {
	const prefix = "arn:aws:sqs:"
	if !strings.HasPrefix(resourceARN, prefix) {
		return "", nil
	}

	// arn:aws:sqs:region:account:queue-name → extract queue name (last segment)
	parts := strings.Split(resourceARN, ":")
	const arnParts = 6
	if len(parts) < arnParts {
		return "", nil
	}

	queueName := parts[len(parts)-1]
	if queueName == "" {
		return "", nil
	}

	accountID := parts[4]
	queueURL := "http://localhost/" + accountID + "/" + queueName

	out, err := a.backend.GetQueueAttributes(&sqsbackend.GetQueueAttributesInput{
		QueueURL:       queueURL,
		AttributeNames: []string{"Policy"},
	})
	if err != nil {
		return "", err
	}

	return out.Attributes["Policy"], nil
}

// kmsPolicyAdapter wraps a KMS backend to implement ResourcePolicyProvider.
type kmsPolicyAdapter struct {
	backend kmsPolicyBackend
}

func (a *kmsPolicyAdapter) GetResourcePolicy(
	ctx context.Context,
	resourceARN string,
) (string, error) {
	const prefix = "arn:aws:kms:"
	if !strings.HasPrefix(resourceARN, prefix) {
		return "", nil
	}

	out, err := a.backend.GetKeyPolicy(ctx, &kmsbackend.GetKeyPolicyInput{KeyID: resourceARN})
	if err != nil {
		return "", err
	}

	return out.Policy, nil
}

// secretsManagerPolicyAdapter wraps a Secrets Manager backend to implement ResourcePolicyProvider.
type secretsManagerPolicyAdapter struct {
	backend secretsManagerPolicyBackend
}

func (a *secretsManagerPolicyAdapter) GetResourcePolicy(
	ctx context.Context,
	resourceARN string,
) (string, error) {
	const prefix = "arn:aws:secretsmanager:"
	if !strings.HasPrefix(resourceARN, prefix) {
		return "", nil
	}

	out, err := a.backend.GetResourcePolicy(
		ctx,
		&secretsmanagerbackend.GetResourcePolicyInput{SecretID: resourceARN},
	)
	if err != nil {
		return "", err
	}

	return out.ResourcePolicy, nil
}

// lambdaPolicyAdapter wraps a Lambda backend to implement ResourcePolicyProvider.
type lambdaPolicyAdapter struct {
	backend lambdaPolicyBackend
}

func (a *lambdaPolicyAdapter) GetResourcePolicy(
	_ context.Context,
	resourceARN string,
) (string, error) {
	const prefix = "arn:aws:lambda:"
	if !strings.HasPrefix(resourceARN, prefix) {
		return "", nil
	}

	parts := strings.Split(resourceARN, ":")
	const arnMinParts = 6
	if len(parts) < arnMinParts {
		return "", nil
	}

	fnName := strings.TrimPrefix(parts[5], "function:")
	if fnName == "" {
		return "", nil
	}

	out, err := a.backend.GetPolicy(fnName, "")
	if err != nil || out == nil || out.Policy == nil {
		return "", err
	}

	return *out.Policy, nil
}

// ecrPolicyAdapter wraps an ECR backend to implement ResourcePolicyProvider.
type ecrPolicyAdapter struct {
	backend ecrPolicyBackend
}

func (a *ecrPolicyAdapter) GetResourcePolicy(
	ctx context.Context,
	resourceARN string,
) (string, error) {
	const prefix = "arn:aws:ecr:"
	if !strings.HasPrefix(resourceARN, prefix) {
		return "", nil
	}

	parts := strings.Split(resourceARN, ":")
	const arnMinParts = 6
	if len(parts) < arnMinParts {
		return "", nil
	}

	repoName := strings.TrimPrefix(parts[5], "repository/")
	if repoName == "" {
		return "", nil
	}

	out, err := a.backend.GetRepositoryPolicy(ctx, repoName)
	if err != nil || out == nil {
		return "", err
	}

	return out.PolicyText, nil
}

// snsPolicyAdapter wraps a SNS backend to implement ResourcePolicyProvider.
type snsPolicyAdapter struct {
	backend snsPolicyBackend
}

func (a *snsPolicyAdapter) GetResourcePolicy(
	_ context.Context,
	resourceARN string,
) (string, error) {
	const prefix = "arn:aws:sns:"
	if !strings.HasPrefix(resourceARN, prefix) {
		return "", nil
	}

	attrs, err := a.backend.GetTopicAttributes(resourceARN)
	if err != nil {
		return "", err
	}

	return attrs["Policy"], nil
}

// ebPolicyAdapter wraps an EventBridge backend to implement ResourcePolicyProvider.
type ebPolicyAdapter struct {
	backend ebPolicyBackend
}

func (a *ebPolicyAdapter) GetResourcePolicy(
	ctx context.Context,
	resourceARN string,
) (string, error) {
	const prefix = "arn:aws:events:"
	if !strings.HasPrefix(resourceARN, prefix) {
		return "", nil
	}

	parts := strings.Split(resourceARN, ":")
	const arnMinParts = 6
	if len(parts) < arnMinParts {
		return "", nil
	}

	busName := strings.TrimPrefix(parts[5], "event-bus/")
	if busName == "" {
		busName = "default"
	}

	bus, err := a.backend.DescribeEventBus(ctx, busName)
	if err != nil || bus == nil {
		return "", err
	}

	return bus.Policy, nil
}

// bedrockPolicyAdapter wraps a Bedrock backend to implement ResourcePolicyProvider.
type bedrockPolicyAdapter struct {
	backend bedrockPolicyBackend
}

func (a *bedrockPolicyAdapter) GetResourcePolicy(
	_ context.Context,
	resourceARN string,
) (string, error) {
	const prefix = "arn:aws:bedrock:"
	if !strings.HasPrefix(resourceARN, prefix) {
		return "", nil
	}

	rp, err := a.backend.GetResourcePolicy(resourceARN)
	if err != nil || rp == nil {
		return "", err
	}

	return rp.PolicyDocument, nil
}

// startEmbeddedDNS creates and starts the embedded DNS server.
// Configuration errors and startup failures are logged as warnings; the server
// continues to run without DNS in those cases.
func startEmbeddedDNS(ctx context.Context, addr, resolveIP string) *gopherDNS.Server {
	log := logger.Load(ctx)

	dnsSrv, err := gopherDNS.New(gopherDNS.Config{
		ListenAddr: addr,
		ResolveIP:  resolveIP,
		Logger:     log,
	})
	if err != nil {
		log.WarnContext(ctx, "DNS server disabled (config error)", "error", err)

		return nil
	}

	if startErr := dnsSrv.Start(ctx); startErr != nil {
		log.WarnContext(ctx, "DNS server failed to start", "error", startErr)

		return nil
	}

	log.InfoContext(ctx, "DNS server started", "addr", addr)

	return dnsSrv
}

// wireLambdaDNS sets the DNS registrar on the Lambda backend so function URL
// hostnames are automatically registered when CreateFunctionUrlConfig is called.
func wireLambdaDNS(lambdaReg service.Registerable, dns lambdabackend.DNSRegistrar) {
	if lambdaReg == nil || dns == nil {
		return
	}

	lambdaH, ok := lambdaReg.(*lambdabackend.Handler)
	if !ok {
		return
	}

	if lambdaBk, bkOk := lambdaH.Backend.(*lambdabackend.InMemoryBackend); bkOk {
		lambdaBk.SetDNSRegistrar(dns)
	}
}

// wireRoute53DNS sets the DNS registrar on the Route 53 backend so that
// A and CNAME record sets are automatically registered in the embedded DNS server.
func wireRoute53DNS(r53Reg service.Registerable, dns route53backend.DNSRegistrar) {
	if r53Reg == nil || dns == nil {
		return
	}

	r53H, ok := r53Reg.(*route53backend.Handler)
	if !ok {
		return
	}

	bk, isMem := r53H.Backend.(*route53backend.InMemoryBackend)
	if isMem {
		bk.SetDNSRegistrar(dns)
	}
}

// wireRDSDNS sets the DNS registrar on the RDS backend so that instance hostnames
// are automatically registered with the embedded DNS server.
func wireRDSDNS(rdsReg service.Registerable, dns rdsbackend.DNSRegistrar) {
	if rdsReg == nil || dns == nil {
		return
	}

	rdsH, ok := rdsReg.(*rdsbackend.Handler)
	if !ok {
		return
	}

	rdsH.Backend.SetDNSRegistrar(dns)
}

// wireRedshiftDNS sets the DNS registrar on the Redshift backend so that cluster
// hostnames are automatically registered with the embedded DNS server.
func wireRedshiftDNS(redshiftReg service.Registerable, dns redshiftbackend.DNSRegistrar) {
	if redshiftReg == nil || dns == nil {
		return
	}

	redshiftH, ok := redshiftReg.(*redshiftbackend.Handler)
	if !ok {
		return
	}

	redshiftH.Backend.SetDNSRegistrar(dns)
}

// wireOpenSearchDNS sets the DNS registrar on the OpenSearch backend so that domain
// hostnames are automatically registered with the embedded DNS server.
func wireOpenSearchDNS(osReg service.Registerable, dns opensearchbackend.DNSRegistrar) {
	if osReg == nil || dns == nil {
		return
	}

	osH, ok := osReg.(*opensearchbackend.Handler)
	if !ok {
		return
	}

	bk, ok := osH.Backend.(*opensearchbackend.InMemoryBackend)
	if !ok {
		return
	}

	bk.SetDNSRegistrar(dns)
}

// wireElasticsearchDNS sets the DNS registrar on the Elasticsearch backend so that domain
// hostnames are automatically registered with the embedded DNS server.
func wireElasticsearchDNS(esReg service.Registerable, dns elasticsearchbackend.DNSRegistrar) {
	if esReg == nil || dns == nil {
		return
	}

	esH, ok := esReg.(*elasticsearchbackend.Handler)
	if !ok {
		return
	}

	esH.Backend.SetDNSRegistrar(dns)
}

// wireElastiCacheDNS sets the DNS registrar on the ElastiCache backend so
// cache cluster endpoints use AWS-style hostnames registered in the embedded DNS.
func wireElastiCacheDNS(ecReg service.Registerable, dns elasticachebackend.DNSRegistrar) {
	if ecReg == nil || dns == nil {
		return
	}

	ecH, ok := ecReg.(*elasticachebackend.Handler)
	if !ok {
		return
	}

	if ecBk, bkOk := ecH.Backend.(*elasticachebackend.InMemoryBackend); bkOk {
		ecBk.SetDNSRegistrar(dns)
	}
}

// wireEC2DNS sets the DNS registrar on the EC2 backend so that the synthetic
// public DNS hostnames generated by the docker compute provider can be
// resolved from outside the gopherstack process.
func wireEC2DNS(ec2Reg service.Registerable, dns ec2backend.DNSRegistrar) {
	if ec2Reg == nil || dns == nil {
		return
	}

	ec2H, ok := ec2Reg.(*ec2backend.Handler)
	if !ok {
		return
	}

	if ec2Bk, bkOk := ec2H.Backend.(*ec2backend.InMemoryBackend); bkOk {
		ec2Bk.SetDNSRegistrar(dns)
	}
}

// wireFirehoseDelivery connects the Firehose backend to S3 and Lambda so that
// buffered records are delivered to the configured S3 bucket, and optionally
// transformed by a Lambda function before delivery.
func wireFirehoseDelivery(firehoseReg, s3Reg, lambdaReg service.Registerable) {
	firehoseH, ok := firehoseReg.(*firehosebackend.Handler)
	if !ok {
		return
	}

	if s3H, s3Ok := s3Reg.(*s3backend.S3Handler); s3Ok {
		if s3Bk, bkOk := s3H.Backend.(*s3backend.InMemoryBackend); bkOk {
			if fhBk, fhOk := firehoseH.Backend.(*firehosebackend.InMemoryBackend); fhOk {
				fhBk.SetS3Backend(s3Bk)
			}
		}
	}

	if lambdaH, lambdaOk := lambdaReg.(*lambdabackend.Handler); lambdaOk {
		if lambdaBk, bkOk := lambdaH.Backend.(*lambdabackend.InMemoryBackend); bkOk {
			if fhBk, fhOk := firehoseH.Backend.(*firehosebackend.InMemoryBackend); fhOk {
				fhBk.SetLambdaBackend(lambdaBk)
			}
		}
	}
}

// wireDynamoDBS3 connects the DynamoDB backend to the S3 backend so that
// ImportTable can read source objects and ExportTableToPointInTime can write
// export data to S3.
func wireDynamoDBS3(ddbReg, s3Reg service.Registerable) {
	ddbH, ok := ddbReg.(*ddbbackend.DynamoDBHandler)
	if !ok {
		return
	}

	s3H, s3Ok := s3Reg.(*s3backend.S3Handler)
	if !s3Ok {
		return
	}

	s3Bk, bkOk := s3H.Backend.(*s3backend.InMemoryBackend)
	if !bkOk {
		return
	}

	if ddbBk, ddbBkOk := ddbH.Backend.(*ddbbackend.InMemoryDB); ddbBkOk {
		ddbBk.SetS3Backend(s3Bk)
	}
}

// wireMGNS3 connects the MGN backend to the S3 backend so StartImport can
// read its caller-supplied S3 object and actually create SourceServers.
func wireMGNS3(mgnReg, s3Reg service.Registerable) {
	mgnH, ok := mgnReg.(*mgnbackend.Handler)
	if !ok {
		return
	}

	s3H, s3Ok := s3Reg.(*s3backend.S3Handler)
	if !s3Ok {
		return
	}

	s3Bk, bkOk := s3H.Backend.(*s3backend.InMemoryBackend)
	if !bkOk || mgnH.Backend == nil {
		return
	}

	mgnH.Backend.SetS3Backend(s3Bk)
}

// wireSageMakerS3 connects the SageMaker backend to the S3 backend so
// CreatePipeline/UpdatePipeline can fetch a PipelineDefinitionS3Location's
// object and use it as the real pipeline definition instead of failing the
// request outright.
func wireSageMakerS3(smReg, s3Reg service.Registerable) {
	smH, ok := smReg.(*sagemakerbackend.Handler)
	if !ok {
		return
	}

	s3H, s3Ok := s3Reg.(*s3backend.S3Handler)
	if !s3Ok {
		return
	}

	s3Bk, bkOk := s3H.Backend.(*s3backend.InMemoryBackend)
	if !bkOk || smH.Backend == nil {
		return
	}

	smH.Backend.SetS3Backend(s3Bk)
}

// wireGlacierS3 connects the Glacier backend to the S3 backend so a completed
// Select job writes its real OutputLocation output (job.txt/results/
// result_manifest.txt, see services/glacier/select_output.go) instead of only
// serving results via GetJobOutput.
func wireGlacierS3(glacierReg, s3Reg service.Registerable) {
	glH, ok := glacierReg.(*glacierbackend.Handler)
	if !ok {
		return
	}

	s3H, s3Ok := s3Reg.(*s3backend.S3Handler)
	if !s3Ok {
		return
	}

	s3Bk, bkOk := s3H.Backend.(*s3backend.InMemoryBackend)
	if !bkOk {
		return
	}

	glBk, glBkOk := glH.Backend.(*glacierbackend.InMemoryBackend)
	if !glBkOk {
		return
	}

	glBk.SetS3Backend(s3Bk)
}

// iotAnalyticsThingRegistryAdapter adapts the IoT backend's DescribeThing to the
// iotanalytics.ThingRegistry interface for the "deviceRegistryEnrich" pipeline activity
// (iot:DescribeThing, per the CloudFormation docs for AWS::IoTAnalytics::Pipeline
// DeviceRegistryEnrich's RoleArn requirement).
type iotAnalyticsThingRegistryAdapter struct {
	backend *iotbackend.InMemoryBackend
}

func (a *iotAnalyticsThingRegistryAdapter) DescribeThing(thingName string) (map[string]any, error) {
	t, err := a.backend.DescribeThing(thingName)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"thingName":     t.ThingName,
		"thingId":       t.ThingID,
		"thingArn":      t.ARN,
		"thingTypeName": t.ThingTypeName,
		"attributes":    t.Attributes,
		"version":       t.Version,
	}, nil
}

// iotAnalyticsThingShadowAdapter adapts the IoT backend's GetThingShadow (classic shadow) to
// the iotanalytics.ThingShadowStore interface for the "deviceShadowEnrich" pipeline activity
// (iot:GetThingShadow).
type iotAnalyticsThingShadowAdapter struct {
	backend *iotbackend.InMemoryBackend
}

func (a *iotAnalyticsThingShadowAdapter) GetThingShadow(thingName string) (map[string]any, error) {
	s, err := a.backend.GetThingShadow(thingName, "")
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"state":    s.State,
		"metadata": s.Metadata,
		"version":  s.Version,
	}, nil
}

// wireIoTAnalyticsCrossService wires RunPipelineActivity's lambda/deviceRegistryEnrich/
// deviceShadowEnrich activities (services/iotanalytics/pipelines.go) to the real Lambda and
// IoT backends, following the same LambdaInvoker/adapter patterns wireStorageAndSecretsIntegrations
// already uses for SNS, Firehose, and SecretsManager.
func wireIoTAnalyticsCrossService(iotaReg, lambdaReg, iotReg service.Registerable) {
	iotaH, ok := iotaReg.(*iotanalyticsbackend.Handler)
	if !ok {
		return
	}

	iotaBk, bkOk := iotaH.Backend.(*iotanalyticsbackend.InMemoryBackend)
	if !bkOk {
		return
	}

	if lambdaH, lambdaOk := lambdaReg.(*lambdabackend.Handler); lambdaOk {
		if lambdaBk, lbkOk := lambdaH.Backend.(*lambdabackend.InMemoryBackend); lbkOk {
			iotaBk.SetLambdaBackend(lambdaBk)
		}
	}

	iotH, iotOk := iotReg.(*iotbackend.Handler)
	if !iotOk {
		return
	}

	iotBk, ibkOk := iotH.Backend.(*iotbackend.InMemoryBackend)
	if !ibkOk {
		return
	}

	iotaBk.SetThingRegistry(&iotAnalyticsThingRegistryAdapter{backend: iotBk})
	iotaBk.SetThingShadowStore(&iotAnalyticsThingShadowAdapter{backend: iotBk})
}

// kinesisAnalyticsStreamReaderAdapter adapts the Kinesis backend's real
// ListShards/GetShardIterator/GetRecords (ctx+typed-struct shaped, see
// services/kinesis/records.go and shards.go) to
// kinesisanalyticsbackend.KinesisStreamReader's narrow (streamName string, limit int) shape
// DiscoverInputSchema samples through (services/kinesisanalytics/discover_schema.go).
type kinesisAnalyticsStreamReaderAdapter struct {
	backend *kinesisbackend.InMemoryBackend
}

// kaTrimHorizonIteratorType is the only shard-iterator starting point DiscoverInputSchema's
// sampling needs (it just wants some records, not a caller-specified position). Not
// kinesisbackend.iteratorTypeTrimHorizon -- that constant is unexported (services/kinesis/
// models.go:40).
const kaTrimHorizonIteratorType = "TRIM_HORIZON"

func (a *kinesisAnalyticsStreamReaderAdapter) ListShards(streamName string) ([]string, error) {
	out, err := a.backend.ListShards(context.Background(), &kinesisbackend.ListShardsInput{StreamName: streamName})
	if err != nil {
		return nil, err
	}

	ids := make([]string, len(out.Shards))
	for i, s := range out.Shards {
		ids[i] = s.ShardID
	}

	return ids, nil
}

func (a *kinesisAnalyticsStreamReaderAdapter) GetShardIterator(streamName, shardID string) (string, error) {
	out, err := a.backend.GetShardIterator(context.Background(), &kinesisbackend.GetShardIteratorInput{
		StreamName:        streamName,
		ShardID:           shardID,
		ShardIteratorType: kaTrimHorizonIteratorType,
	})
	if err != nil {
		return "", err
	}

	return out.ShardIterator, nil
}

func (a *kinesisAnalyticsStreamReaderAdapter) GetRecords(
	shardIterator string,
	limit int,
) ([][]byte, string, error) {
	out, err := a.backend.GetRecords(context.Background(), &kinesisbackend.GetRecordsInput{
		ShardIterator: shardIterator,
		Limit:         limit,
	})
	if err != nil {
		return nil, "", err
	}

	records := make([][]byte, len(out.Records))
	for i, r := range out.Records {
		records[i] = r.Data
	}

	return records, out.NextShardIterator, nil
}

// wireKinesisAnalyticsCrossService wires DiscoverInputSchema's real sampling
// (services/kinesisanalytics/discover_schema.go) to the Kinesis and S3 backends.
// s3backend.InMemoryBackend.GetObject satisfies kinesisanalyticsbackend.S3ObjectReader
// directly (same real SDK types, no adapter -- the same no-adapter pairing as cloudwatch's
// FirehosePutter/firehose.InMemoryBackend). Kinesis needs
// kinesisAnalyticsStreamReaderAdapter to bridge onto KinesisStreamReader's narrow shape.
func wireKinesisAnalyticsCrossService(kaReg, kinesisReg, s3Reg service.Registerable) {
	kaH, ok := kaReg.(*kinesisanalyticsbackend.Handler)
	if !ok {
		return
	}

	kaBk, bkOk := kaH.Backend.(*kinesisanalyticsbackend.InMemoryBackend)
	if !bkOk {
		return
	}

	if kinesisH, kOk := kinesisReg.(*kinesisbackend.Handler); kOk {
		if kinesisBk, kbkOk := kinesisH.Backend.(*kinesisbackend.InMemoryBackend); kbkOk {
			kaBk.SetKinesisStreamReader(&kinesisAnalyticsStreamReaderAdapter{backend: kinesisBk})
		}
	}

	s3H, s3Ok := s3Reg.(*s3backend.S3Handler)
	if !s3Ok {
		return
	}

	s3Bk, s3BkOk := s3H.Backend.(*s3backend.InMemoryBackend)
	if !s3BkOk {
		return
	}

	kaBk.SetS3ObjectReader(s3Bk)
}

// cfnLightsailStackAdapter adapts the CloudFormation backend's real
// CreateStack to the lightsailbackend.CloudFormationBackend interface so
// Lightsail's CreateCloudFormationStack (services/lightsail/exportcfn.go)
// hands off to a real services/cloudformation stack instead of always
// taking its honest no-backend-wired fallback (a CloudFormationStackRecord
// with no backing stack ARN). No template body is synthesized -- Lightsail
// gives us instance names, not a CloudFormation template, so fabricating
// resources (AMIs, subnets, etc.) the export didn't actually provide would
// be worse than not wiring this at all. Instead each source instance name
// becomes a stack parameter, and the resulting stack is real: a genuine
// Stack record in the cloudformation backend with a real StackID/ARN.
type cfnLightsailStackAdapter struct {
	backend cfnbackend.StorageBackend
}

func (a *cfnLightsailStackAdapter) CreateStackFromLightsail(stackName string, instanceNames []string) (string, error) {
	params := make([]cfnbackend.Parameter, 0, len(instanceNames))

	for i, name := range instanceNames {
		params = append(params, cfnbackend.Parameter{
			ParameterKey:   fmt.Sprintf("LightsailSourceInstance%d", i+1),
			ParameterValue: name,
		})
	}

	stack, err := a.backend.CreateStack(context.Background(), stackName, "", params, cfnbackend.StackOptions{})
	if err != nil {
		return "", err
	}

	return stack.StackID, nil
}

// wireLightsailCloudFormation connects the Lightsail backend to the
// CloudFormation backend so CreateCloudFormationStack's cross-service
// handoff (services/lightsail/store.go's CloudFormationBackend interface)
// actually fires -- see cfnLightsailStackAdapter. Called from
// registerCloudFormationAndDashboard, not one of the wire*Integrations
// helpers under wireCrossServiceDependencies, because CloudFormation is
// registered after those helpers run.
func wireLightsailCloudFormation(lightsailReg, cfnReg service.Registerable) {
	lightsailH, ok := lightsailReg.(*lightsailbackend.Handler)
	if !ok {
		return
	}

	cfnH, cfnOk := cfnReg.(*cfnbackend.Handler)
	if !cfnOk || lightsailH.Backend == nil || cfnH.Backend == nil {
		return
	}

	lightsailH.Backend.SetCloudFormationBackend(&cfnLightsailStackAdapter{backend: cfnH.Backend})
}

// wireCloudFormationOrganizations wires the Organizations backend as
// CloudFormation's OrganizationsDirectory, so SERVICE_MANAGED StackSet
// operations can resolve DeploymentTargets.OrganizationalUnitIds against the
// real OU tree (services/cloudformation/organizations_directory.go).
func wireCloudFormationOrganizations(cfnReg, orgReg service.Registerable) {
	cfnH, ok := cfnReg.(*cfnbackend.Handler)
	if !ok || cfnH.Backend == nil {
		return
	}

	cfnBk, ok := cfnH.Backend.(*cfnbackend.InMemoryBackend)
	if !ok {
		return
	}

	orgH, ok := orgReg.(*organizationsbackend.Handler)
	if !ok || orgH.Backend == nil {
		return
	}

	orgBk, ok := orgH.Backend.(*organizationsbackend.InMemoryBackend)
	if !ok {
		return
	}

	cfnBk.SetOrganizationsDirectory(orgBk)
}

// extractServiceName finds the service name for a given Echo context by checking
// which service's route matcher matches the request.
func extractServiceName(c *echo.Context, services []service.Registerable) string {
	for _, svc := range services {
		if svc.RouteMatcher()(c) {
			return svc.Name()
		}
	}

	return ""
}

// regionTrackerPersistenceName is the persistence.Manager entry name for the
// region-tracking set (pkgs/service.KnownRegions), distinct from any AWS
// service name.
const regionTrackerPersistenceName = "system-regions"

// regionTrackerPersistable adapts pkgs/service's region tracker to
// persistence.Persistable, so the dashboard's known-region set (see
// /dashboard/api/system/regions) survives a restart.
//
// This persists the tracker's own recorded history rather than trying to
// reconstruct it by scanning other services' restored state: services like
// "account" carry a baked-in region catalog as reference data (not
// resource data), so a byte-scan over their snapshots produces false
// positives indistinguishable from genuine per-region resources.
type regionTrackerPersistable struct{}

func (regionTrackerPersistable) Snapshot(_ context.Context) []byte {
	return service.SnapshotKnownRegions()
}

func (regionTrackerPersistable) Restore(_ context.Context, data []byte) error {
	service.RestoreKnownRegions(data)

	return nil
}

// setupPersistence registers all persistable services with the manager and optionally restores state.
func setupPersistence(
	ctx context.Context,
	m *persistence.Manager,
	services []service.Registerable,
	restore bool,
	defaultRegion string,
) {
	type persistable interface {
		Snapshot(ctx context.Context) []byte
		Restore(context.Context, []byte) error
	}

	for _, svc := range services {
		if p, ok := svc.(persistable); ok {
			m.Register(svc.Name(), p)
		}
	}

	m.Register(regionTrackerPersistenceName, regionTrackerPersistable{})

	// The default region is where a request lands absent an explicit
	// X-Amz-Region header, so it is known from the start rather than
	// waiting for the first live request to touch it.
	service.SeedRegions(defaultRegion)

	if restore {
		m.RestoreAll(ctx)
	}
}

// initPersistenceManager creates and configures a persistence.Manager from the CLI config.
// If persistence is disabled it returns a manager backed by a NullStore.
func initPersistenceManager(ctx context.Context, cli *CLI) (*persistence.Manager, error) {
	log := logger.Load(ctx)
	var store persistence.Store = persistence.NullStore{}

	if cli.Persist && !cli.Demo {
		fs, err := cli.createPersistenceStore()
		if err != nil {
			return nil, fmt.Errorf("persistence: create file store: %w", err)
		}

		store = fs
		log.InfoContext(ctx, "Persistence enabled", "data_dir", cli.resolvedDataDir())
	}

	return persistence.NewManager(ctx, store), nil
}

// loadDemoData loads demo data into the services.
func loadDemoData(ctx context.Context, cli *CLI) {
	log := logger.Load(ctx)
	log.InfoContext(ctx, "Loading demo data...")

	err := demo.LoadData(ctx, &demo.Clients{
		DynamoDB:       cli.ddbClient,
		S3:             cli.s3Client,
		SQS:            cli.sqsClient,
		SNS:            cli.snsClient,
		IAM:            cli.iamClient,
		STS:            cli.stsClient,
		SSM:            cli.ssmClient,
		KMS:            cli.kmsClient,
		SecretsManager: cli.secretsManagerClient,
		ECR:            cli.ecrClient,
		AppSync:        cli.appSyncSdkClient,
		Amplify:        cli.amplifyClient,
		ECS:            cli.ecsClient,
		EKS:            cli.eksClient,
		IoT:            cli.iotClient,
		CodeDeploy:     cli.codeDeployClient,
		CodePipeline:   cli.codePipelineSDKClient,
	})
	if err != nil {
		log.ErrorContext(ctx, "Failed to load demo data", "error", err)
	}

	seedAppConfigDataDemoProfiles(ctx, cli.appConfigDataHandler, log)
	seedBedrockRuntimeDemoInvocations(ctx, cli.bedrockruntimeHandler, log)
	seedRoute53ResolverDemoData(ctx, cli.route53resolverHandler, log)
	seedEMRServerlessDemoData(ctx, cli.emrserverlessHandler, log)
}

// seedAppConfigDataDemoProfiles seeds demo configuration profiles for visual dashboard inspection.
// AppConfigData has no AWS SDK write API, so profiles are seeded directly via the backend.
func seedAppConfigDataDemoProfiles(ctx context.Context, h service.Registerable, log *slog.Logger) {
	acdHandler, ok := h.(*appconfigdatabackend.Handler)
	if !ok || acdHandler == nil {
		log.DebugContext(ctx, "AppConfigData handler not available; skipping demo profile seeding")

		return
	}

	profiles := []struct {
		app, env, profile, content, contentType string
	}{
		{
			app: demoAppName, env: envProduction, profile: "feature-flags",
			content:     `{"featureFlagX":true,"enableNewUI":false,"maxRetries":3}`,
			contentType: contentTypeJSON,
		},
		{
			app: demoAppName, env: envProduction, profile: "rate-limits",
			content:     `{"requestsPerMinute":100,"burstLimit":200}`,
			contentType: contentTypeJSON,
		},
		{
			app: demoAppName, env: "staging", profile: "feature-flags",
			content:     `{"featureFlagX":true,"enableNewUI":true,"maxRetries":5}`,
			contentType: contentTypeJSON,
		},
	}

	for _, p := range profiles {
		if err := acdHandler.Backend.SetConfiguration(p.app, p.env, p.profile, p.content, p.contentType); err != nil {
			log.WarnContext(ctx, "Failed to seed AppConfigData profile", "error", err)
		}
	}

	log.InfoContext(ctx, "Seeded AppConfigData demo profiles", "count", len(profiles))
}

// seedBedrockRuntimeDemoInvocations seeds demo invocations for visual dashboard inspection.
// BedrockRuntime has no AWS SDK write API, so invocations are seeded directly via the backend.
func seedBedrockRuntimeDemoInvocations(
	ctx context.Context,
	h service.Registerable,
	log *slog.Logger,
) {
	brtHandler, ok := h.(*bedrockruntimebackend.Handler)
	if !ok || brtHandler == nil {
		log.DebugContext(
			ctx,
			"BedrockRuntime handler not available; skipping demo invocation seeding",
		)

		return
	}

	brtHandler.Backend.RecordInvocation(
		"InvokeModel",
		"anthropic.claude-v2",
		`{"prompt": "Human: What is the capital of France?\n\nAssistant:"}`,
		`{"completion": " Paris is the capital of France.", "stop_reason": "end_turn"}`,
	)
	converseOutput := `{"output": {"message": {"role": "assistant", ` +
		`"content": [{"text": "Hello! How can I help you today?"}]}}, "stopReason": "end_turn"}`
	brtHandler.Backend.RecordInvocation(
		"Converse",
		"anthropic.claude-3-sonnet-20240229-v1:0",
		`{"messages": [{"role": "user", "content": [{"text": "Hello!"}]}]}`,
		converseOutput,
	)

	log.InfoContext(ctx, "Seeded BedrockRuntime demo invocations")
}

// seedRoute53ResolverDemoData seeds demo resources for visual dashboard inspection.
func seedRoute53ResolverDemoData(ctx context.Context, h service.Registerable, log *slog.Logger) {
	r53rHandler, ok := h.(*route53resolverbackend.Handler)
	if !ok || r53rHandler == nil {
		log.DebugContext(ctx, "Route53Resolver handler not available; skipping demo seeding")

		return
	}

	b, ok2 := r53rHandler.Backend.(*route53resolverbackend.InMemoryBackend)
	if !ok2 || b == nil {
		log.DebugContext(
			ctx,
			"Route53Resolver backend type assertion failed; skipping demo seeding",
		)

		return
	}

	ep := b.AddEndpointInternal("corp-inbound", route53resolverbackend.DirectionInbound)
	b.AddEndpointInternal("corp-outbound", route53resolverbackend.DirectionOutbound)
	r := b.AddRuleInternal("corp-forward", "corp.internal.", route53resolverbackend.RuleTypeForward)
	_ = r
	if ep != nil {
		b.AddRuleInternalWithEndpoint(
			"vpc-lookup",
			"internal.example.com.",
			route53resolverbackend.RuleTypeForward,
			ep.ID,
		)
	}
	grp := b.AddFirewallRuleGroupInternal("BlockMalware-Group")
	b.AddFirewallRuleGroupInternal("AlertSuspicious-Group")
	dl := b.AddFirewallDomainListInternal("malware-domains")
	b.AddFirewallDomainListInternal("phishing-domains")
	if grp != nil && dl != nil {
		b.AddFirewallRuleInternal(
			grp.ID,
			"block-malware",
			"BLOCK",
			dl.ID,
			route53resolverbackend.FirewallPriorityDefault,
		)
	}
	b.AddQueryLogConfigInternal(
		"vpc-query-logs",
		"arn:aws:logs:us-east-1:000000000000:log-group:/resolver/queries",
	)

	log.InfoContext(ctx, "Seeded Route53Resolver demo data")
}

// seedEMRServerlessDemoData seeds demo applications and job runs for visual dashboard inspection.
func seedEMRServerlessDemoData(ctx context.Context, h service.Registerable, log *slog.Logger) {
	emrHandler, ok := h.(*emrserverlessbackend.Handler)
	if !ok || emrHandler == nil {
		log.DebugContext(ctx, "EMR Serverless handler not available; skipping demo seeding")

		return
	}

	now := time.Now().UTC()

	// Create demo applications.
	spark := &emrserverlessbackend.Application{
		ApplicationID: "demo0spark1",
		Arn:           "arn:aws:emr-serverless:us-east-1:000000000000:/applications/demo0spark1",
		Name:          "spark-etl-app",
		Type:          "SPARK",
		ReleaseLabel:  "emr-6.10.0",
		State:         emrserverlessbackend.ApplicationStateStarted,
		Tags:          map[string]string{"team": "data-engineering", "env": envProduction},
		CreatedAt:     now.Add(-72 * time.Hour),
		UpdatedAt:     now.Add(-1 * time.Hour),
	}
	hive := &emrserverlessbackend.Application{
		ApplicationID: "demo0hive01",
		Arn:           "arn:aws:emr-serverless:us-east-1:000000000000:/applications/demo0hive01",
		Name:          "hive-analytics-app",
		Type:          "HIVE",
		ReleaseLabel:  "emr-6.8.0",
		State:         emrserverlessbackend.ApplicationStateStopped,
		Tags:          map[string]string{"team": "analytics"},
		CreatedAt:     now.Add(-168 * time.Hour),
		UpdatedAt:     now.Add(-48 * time.Hour),
	}
	emrHandler.Backend.AddApplicationInternal(spark)
	emrHandler.Backend.AddApplicationInternal(hive)

	// Create demo job runs for the Spark application.
	emrHandler.Backend.AddJobRunInternal(&emrserverlessbackend.JobRun{
		ApplicationID:    spark.ApplicationID,
		JobRunID:         "demo0jr001",
		Arn:              "arn:aws:emr-serverless:us-east-1:000000000000:/applications/demo0spark1/jobruns/demo0jr001",
		Name:             "daily-etl-pipeline",
		State:            emrserverlessbackend.JobRunStateSuccess,
		ExecutionRoleArn: emrServerlessRoleARN,
		CreatedAt:        now.Add(-24 * time.Hour),
		UpdatedAt:        now.Add(-23 * time.Hour),
		Tags:             map[string]string{"run": "daily"},
	})
	emrHandler.Backend.AddJobRunInternal(&emrserverlessbackend.JobRun{
		ApplicationID:    spark.ApplicationID,
		JobRunID:         "demo0jr002",
		Arn:              "arn:aws:emr-serverless:us-east-1:000000000000:/applications/demo0spark1/jobruns/demo0jr002",
		Name:             "hourly-aggregation",
		State:            emrserverlessbackend.JobRunStateSubmitted,
		ExecutionRoleArn: emrServerlessRoleARN,
		CreatedAt:        now.Add(-15 * time.Minute),
		UpdatedAt:        now.Add(-15 * time.Minute),
		Tags:             map[string]string{"run": "hourly"},
	})
	emrHandler.Backend.AddJobRunInternal(&emrserverlessbackend.JobRun{
		ApplicationID:    spark.ApplicationID,
		JobRunID:         "demo0jr003",
		Arn:              "arn:aws:emr-serverless:us-east-1:000000000000:/applications/demo0spark1/jobruns/demo0jr003",
		Name:             "backfill-2024-q1",
		State:            emrserverlessbackend.JobRunStateFailed,
		StateDetails:     "SparkContext stopped due to OOM in executor",
		ExecutionRoleArn: emrServerlessRoleARN,
		CreatedAt:        now.Add(-48 * time.Hour),
		UpdatedAt:        now.Add(-47 * time.Hour),
		Tags:             map[string]string{},
	})
	emrHandler.Backend.AddJobRunInternal(&emrserverlessbackend.JobRun{
		ApplicationID:    spark.ApplicationID,
		JobRunID:         "demo0jr004",
		Arn:              "arn:aws:emr-serverless:us-east-1:000000000000:/applications/demo0spark1/jobruns/demo0jr004",
		Name:             "schema-migration-v2",
		State:            emrserverlessbackend.JobRunStateCancelled,
		StateDetails:     "Job run cancelled by user request",
		ExecutionRoleArn: emrServerlessRoleARN,
		CreatedAt:        now.Add(-12 * time.Hour),
		UpdatedAt:        now.Add(-12 * time.Hour),
		Tags:             map[string]string{},
	})

	const seededApps, seededJobRuns = 2, 4

	log.InfoContext(
		ctx,
		"Seeded EMR Serverless demo data",
		"applications",
		seededApps,
		"jobRuns",
		seededJobRuns,
	)
}

// in HTTP handlers, logs the panic and stack trace via slog, and returns an
// HTTP 500 response. Without this middleware, a single unhandled panic in any
// handler goroutine kills the entire server process immediately.
func panicRecoveryMiddleware() echo.MiddlewareFunc {
	const stackSize = 4 << 10 // 4 KB

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) (err error) {
			defer func() {
				if r := recover(); r != nil {
					// Re-panic for http.ErrAbortHandler — it is not a real error
					// and must propagate to signal the transport layer.
					if rErr, ok := r.(error); ok && errors.Is(rErr, http.ErrAbortHandler) {
						panic(r)
					}

					panicErr, ok := r.(error)
					if !ok {
						panicErr = recoveredPanicError{val: r}
					}

					stack := make([]byte, stackSize)
					n := runtime.Stack(stack, false)
					logger.Load(c.Request().Context()).Error("panic recovered in HTTP handler",
						"error", panicErr, "stack", string(stack[:n]))

					// Record a metric so dashboards can alert on panic spikes.
					telemetry.RecordWorkerTask("http", "PanicRecovery", "error")

					err = c.JSON(http.StatusInternalServerError, map[string]string{
						"message": "internal server error",
					})
				}
			}()

			return next(c)
		}
	}
}

// awsMetaMiddleware populates the per-request AWS metadata ctxbag (account,
// region, partition, request ID) and threads the same fields onto the context
// logger so every record emitted via logger.Load(ctx) is tagged uniformly.
// Backends read identity through awsmeta.Region(ctx)/awsmeta.Account(ctx)
// rather than re-deriving it from the raw request, giving every service a
// single, consistent source of request-scoped metadata and logging.
func awsMetaMiddleware(defaultRegion, defaultAccount string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			req := c.Request()
			meta := awsmeta.FromRequest(req, defaultRegion)

			// FromRequest defaults the account to awsmeta.DefaultAccount; honor
			// the operator-configured account when no per-request override was
			// supplied via the X-Amz-Account-Id header.
			if meta.Account == awsmeta.DefaultAccount && defaultAccount != "" {
				meta.Account = defaultAccount
			}

			// The request-id is set on the response (not the request) by
			// RequestIDMiddleware; carry it through so logs and metadata agree.
			if meta.RequestID == "" {
				meta.RequestID = c.Response().Header().Get("X-Amz-Request-Id")
			}

			ctx := awsmeta.Set(req.Context(), meta)
			ctx = logger.AddAttrs(
				ctx,
				slog.String("region", meta.Region),
				slog.String("account", meta.Account),
				slog.String("request_id", meta.RequestID),
			)
			c.SetRequest(req.WithContext(ctx))

			return next(c)
		}
	}
}

// recoveredPanicError wraps a non-error panic value recovered by panicRecoveryMiddleware.
type recoveredPanicError struct {
	val any
}

func (e recoveredPanicError) Error() string {
	return fmt.Sprintf("panic: %v", e.val)
}

// persistenceMiddleware returns an Echo middleware that schedules a debounced snapshot
// after each mutating request.
func persistenceMiddleware(
	m *persistence.Manager,
	services []service.Registerable,
) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			handlerErr := next(c)

			if isMutatingMethod(c.Request().Method) {
				if svcName := extractServiceName(c, services); svcName != "" {
					m.Notify(svcName)
				}
			}

			return handlerErr
		}
	}
}

// isMutatingMethod reports whether the HTTP method is a state-mutating method.
func isMutatingMethod(method string) bool {
	return method == http.MethodPost || method == http.MethodPut ||
		method == http.MethodPatch || method == http.MethodDelete
}

// wireFISFaultStore injects the chaos FaultStore into the FIS backend so that
// aws:fis:inject-api-* actions can create and remove fault rules during experiments.
func wireFISFaultStore(fisReg service.Registerable, store *chaos.FaultStore) {
	if fisReg == nil || store == nil {
		return
	}

	// Use type assertion to reach the FIS handler's SetFaultStore method.
	if h, ok := fisReg.(interface {
		SetFaultStore(*chaos.FaultStore)
	}); ok {
		h.SetFaultStore(store)
	}
}

// wireFISActionProviders collects all services implementing service.FISActionProvider
// and registers them with the FIS backend for auto-discovered action execution.
func wireFISActionProviders(fisReg service.Registerable, services []service.Registerable) {
	if fisReg == nil {
		return
	}

	type actionProviderSetter interface {
		SetActionProviders([]service.FISActionProvider)
	}

	setter, ok := fisReg.(actionProviderSetter)
	if !ok {
		return
	}

	var providers []service.FISActionProvider

	for _, svc := range services {
		if p, pOK := svc.(service.FISActionProvider); pOK {
			providers = append(providers, p)
		}
	}

	setter.SetActionProviders(providers)
}

// wireCloudFrontKeyValueStore connects the CloudFront KeyValueStore data-plane
// handler to the CloudFront in-memory backend, mirroring wireDynamoDBStreams
// below: the KVS data-plane ops belong to a separate SDK module/protocol
// (gopherstack-4ara) but act on the same KeyValueStore state CloudFront's own
// control-plane ops (CreateKeyValueStore etc.) manage, so the handler is wired
// directly to CloudFront's backend rather than owning a duplicate store.
func wireCloudFrontKeyValueStore(cfReg, kvsReg service.Registerable) {
	kvsH, ok := kvsReg.(*cfkvsbackend.Handler)
	if !ok {
		return
	}

	cfH, cfOk := cfReg.(*cloudfrontbackend.Handler)
	if !cfOk {
		return
	}

	kvsH.Backend = cfH.Backend
}

// wireDynamoDBStreams connects the DynamoDB Streams handler to the DynamoDB in-memory backend
// so that both services share the same underlying stream state.
func wireDynamoDBStreams(ddbReg, streamsReg service.Registerable) {
	streamsH, ok := streamsReg.(*dynamodbstreamsbackend.Handler)
	if !ok {
		return
	}

	ddbH, ddbOk := ddbReg.(*ddbbackend.DynamoDBHandler)
	if !ddbOk {
		return
	}

	if ddbBk, bkOk := ddbH.Backend.(ddbbackend.StreamsBackend); bkOk {
		streamsH.Streams = ddbBk
	}

	streamsH.DefaultRegion = ddbH.DefaultRegion
}

// wireSchedulerRunner configures the Scheduler runner with Lambda, SQS, SNS, and StepFunctions
// target invokers so that schedule expressions actually fire their targets.
func wireSchedulerRunner(
	schedReg, lambdaReg, sqsReg, snsReg, sfnReg, ebReg, kinesisReg, sagemakerReg, ecsReg service.Registerable,
) {
	schedH, ok := schedReg.(*schedulerbackend.Handler)
	if !ok {
		return
	}

	runner := schedH.GetRunner()
	wireSchedulerMessaging(runner, lambdaReg, sqsReg, snsReg)
	wireSchedulerWorkflow(runner, sfnReg, ebReg, kinesisReg)
	wireSchedulerCompute(runner, sagemakerReg, ecsReg)
}

func wireSchedulerMessaging(
	runner *schedulerbackend.Runner,
	lambdaReg, sqsReg, snsReg service.Registerable,
) {
	if lambdaH, ok := lambdaReg.(*lambdabackend.Handler); ok {
		if lambdaBk, ok2 := lambdaH.Backend.(*lambdabackend.InMemoryBackend); ok2 {
			runner.SetLambdaInvoker(&schedulerLambdaAdapter{backend: lambdaBk})
		}
	}

	if sqsH, ok := sqsReg.(*sqsbackend.Handler); ok {
		if sqsBk, ok2 := sqsH.Backend.(*sqsbackend.InMemoryBackend); ok2 {
			runner.SetSQSSender(&sqsSenderAdapter{backend: sqsBk})
		}
	}

	if snsH, ok := snsReg.(*snsbackend.Handler); ok {
		if snsBk, ok2 := snsH.Backend.(*snsbackend.InMemoryBackend); ok2 {
			runner.SetSNSPublisher(&snsPublisherAdapter{backend: snsBk})
		}
	}
}

func wireSchedulerWorkflow(
	runner *schedulerbackend.Runner,
	sfnReg, ebReg, kinesisReg service.Registerable,
) {
	if sfnH, ok := sfnReg.(*sfnbackend.Handler); ok {
		if sfnBk, ok2 := sfnH.Backend.(*sfnbackend.InMemoryBackend); ok2 {
			runner.SetStepFunctionsStarter(&sfnStarterAdapter{backend: sfnBk})
		}
	}

	if ebH, ok := ebReg.(*ebbackend.Handler); ok {
		if ebBk, ok2 := ebH.Backend.(*ebbackend.InMemoryBackend); ok2 {
			runner.SetEventBusPutter(&schedEventBusAdapter{backend: ebBk})
		}
	}

	if kinesisH, ok := kinesisReg.(*kinesisbackend.Handler); ok {
		if kinesisBk, ok2 := kinesisH.Backend.(*kinesisbackend.InMemoryBackend); ok2 {
			runner.SetKinesisRecordPutter(&schedKinesisAdapter{backend: kinesisBk})
		}
	}
}

func wireSchedulerCompute(
	runner *schedulerbackend.Runner,
	sagemakerReg, ecsReg service.Registerable,
) {
	if sagemakerH, ok := sagemakerReg.(*sagemakerbackend.Handler); ok {
		if sagemakerBk := sagemakerH.Backend; sagemakerBk != nil {
			runner.SetSageMakerPipelineStarter(&schedSageMakerAdapter{backend: sagemakerBk})
		}
	}

	if ecsH, ok := ecsReg.(*ecsbackend.Handler); ok {
		if ecsBk, ok2 := ecsH.Backend.(*ecsbackend.InMemoryBackend); ok2 {
			runner.SetECSTaskRunner(&schedECSAdapter{backend: ecsBk})
		}
	}
}

// schedulerLambdaAdapter adapts the Lambda backend to the scheduler.LambdaInvoker interface.
type schedulerLambdaAdapter struct {
	backend *lambdabackend.InMemoryBackend
}

func (a *schedulerLambdaAdapter) InvokeFunction(
	ctx context.Context,
	name, invocationType string,
	payload []byte,
) ([]byte, int, error) {
	return a.backend.InvokeFunction(ctx, name, invocationType, payload)
}

// sfnStarterAdapter adapts the StepFunctions backend to the scheduler.StepFunctionsStarter interface.
type sfnStarterAdapter struct {
	backend *sfnbackend.InMemoryBackend
}

func (a *sfnStarterAdapter) StartExecution(stateMachineARN, name, input string) error {
	_, err := a.backend.StartExecution(stateMachineARN, name, input)

	return err
}

// wirePipesRunner configures the Pipes runner with every source reader (SQS,
// Kinesis, DynamoDB Streams) and every target/DLQ invoker (Lambda,
// StepFunctions, SNS, SQS, Kinesis, EventBridge, CloudWatch Logs, Firehose)
// backed by real service backends, so a RUNNING pipe actually polls its
// source and delivers to its target instead of silently stranding messages
// behind ErrTargetInvokerUnwired. MSK, self-managed Kafka, RabbitMQ, and
// ActiveMQ sources are intentionally left unwired: gopherstack has no
// in-process Kafka-wire-protocol or AMQP/OpenWire broker to poll (see
// services/pipes/PARITY.md and runner.go's pollPipe doc comment).
func wirePipesRunner(
	pipesReg, sqsReg, lambdaReg, sfnReg, snsReg, kinesisReg, ebReg, cwlogsReg, firehoseReg, ddbReg service.Registerable,
) {
	pipesH, ok := pipesReg.(*pipesbackend.Handler)
	if !ok {
		return
	}

	runner := pipesH.GetRunner()

	wirePipesSources(runner, sqsReg, kinesisReg, ddbReg)
	wirePipesInvokers(runner, lambdaReg, sfnReg)
	wirePipesTargets(runner, snsReg, sqsReg, kinesisReg, ebReg, cwlogsReg, firehoseReg)
}

// wirePipesSources wires every pipe source reader backed by a real in-repo
// backend: SQS, Kinesis, and DynamoDB Streams.
func wirePipesSources(runner *pipesbackend.Runner, sqsReg, kinesisReg, ddbReg service.Registerable) {
	if sqsH, sqsOk := sqsReg.(*sqsbackend.Handler); sqsOk {
		if sqsBk, bkOk := sqsH.Backend.(*sqsbackend.InMemoryBackend); bkOk {
			runner.SetSQSReader(&pipesSQSReaderAdapter{backend: sqsBk})
		}
	}

	if kinesisH, kinesisOk := kinesisReg.(*kinesisbackend.Handler); kinesisOk {
		if kinesisBk, bkOk := kinesisH.Backend.(*kinesisbackend.InMemoryBackend); bkOk {
			runner.SetKinesisReader(&pipesKinesisReaderAdapter{backend: kinesisBk})
		}
	}

	if ddbH, ddbOk := ddbReg.(*ddbbackend.DynamoDBHandler); ddbOk {
		if ddbBk, bkOk := ddbH.Backend.(*ddbbackend.InMemoryDB); bkOk {
			runner.SetDynamoDBStreamsReader(&pipesDDBStreamsReaderAdapter{backend: ddbBk})
		}
	}
}

// wirePipesInvokers wires the enrichment/target invokers that call into
// another service's function/execution runtime (Lambda, StepFunctions).
func wirePipesInvokers(runner *pipesbackend.Runner, lambdaReg, sfnReg service.Registerable) {
	if lambdaH, lambdaOk := lambdaReg.(*lambdabackend.Handler); lambdaOk {
		if lambdaBk, bk2Ok := lambdaH.Backend.(*lambdabackend.InMemoryBackend); bk2Ok {
			runner.SetLambdaInvoker(&schedulerLambdaAdapter{backend: lambdaBk})
		}
	}

	if sfnH, sfnOk := sfnReg.(*sfnbackend.Handler); sfnOk {
		if sfnBk, bkOk := sfnH.Backend.(*sfnbackend.InMemoryBackend); bkOk {
			runner.SetStepFunctionsStarter(&pipesSFNStarterAdapter{backend: sfnBk})
		}
	}
}

// wirePipesTargets wires every direct-delivery pipe target (SNS, SQS, Kinesis,
// EventBridge, CloudWatch Logs, Firehose). SNS and SQS double as the two
// supported dead-letter-queue target types (handlePipeFailure/sendToDLQ).
func wirePipesTargets(
	runner *pipesbackend.Runner,
	snsReg, sqsReg, kinesisReg, ebReg, cwlogsReg, firehoseReg service.Registerable,
) {
	if snsH, snsOk := snsReg.(*snsbackend.Handler); snsOk {
		if snsBk, bkOk := snsH.Backend.(*snsbackend.InMemoryBackend); bkOk {
			runner.SetSNSPublisher(&pipesSNSPublisherAdapter{backend: snsBk})
		}
	}

	if sqsH, sqsOk := sqsReg.(*sqsbackend.Handler); sqsOk {
		if sqsBk, bkOk := sqsH.Backend.(*sqsbackend.InMemoryBackend); bkOk {
			runner.SetSQSSender(&pipesSQSSenderAdapter{backend: sqsBk})
		}
	}

	if kinesisH, kinesisOk := kinesisReg.(*kinesisbackend.Handler); kinesisOk {
		if kinesisBk, bkOk := kinesisH.Backend.(*kinesisbackend.InMemoryBackend); bkOk {
			runner.SetKinesisPutter(&pipesKinesisPutterAdapter{backend: kinesisBk})
		}
	}

	if ebH, ebOk := ebReg.(*ebbackend.Handler); ebOk {
		if ebBk, bkOk := ebH.Backend.(*ebbackend.InMemoryBackend); bkOk {
			runner.SetEventBridgePutter(&pipesEventBridgePutterAdapter{backend: ebBk})
		}
	}

	if cwlogsH, cwlogsOk := cwlogsReg.(*cwlogsbackend.Handler); cwlogsOk {
		if cwlogsBk, bkOk := cwlogsH.Backend.(*cwlogsbackend.InMemoryBackend); bkOk {
			runner.SetCloudWatchLogsPutter(&pipesCloudWatchLogsPutterAdapter{backend: cwlogsBk})
		}
	}

	if firehoseH, firehoseOk := firehoseReg.(*firehosebackend.Handler); firehoseOk {
		if firehoseBk, bkOk := firehoseH.Backend.(*firehosebackend.InMemoryBackend); bkOk {
			runner.SetFirehosePutter(&pipesFirehosePutterAdapter{backend: firehoseBk})
		}
	}
}

// pipesSQSReaderAdapter adapts the SQS InMemoryBackend to the pipes.PipeSQSReader interface.
type pipesSQSReaderAdapter struct {
	backend *sqsbackend.InMemoryBackend
}

func (a *pipesSQSReaderAdapter) ReceivePipeMessages(
	queueARN string,
	maxMessages int,
) ([]*pipesbackend.SQSMessage, error) {
	url := arnToSQSQueueURL(queueARN)

	msgs, err := a.backend.ReceiveMessagesLocal(url, maxMessages)
	if err != nil {
		return nil, err
	}

	result := make([]*pipesbackend.SQSMessage, len(msgs))
	for i, m := range msgs {
		result[i] = &pipesbackend.SQSMessage{
			MessageID:     m.MessageID,
			ReceiptHandle: m.ReceiptHandle,
			Body:          m.Body,
			Attributes:    m.Attributes,
			MD5OfBody:     m.MD5OfBody,
		}
	}

	return result, nil
}

func (a *pipesSQSReaderAdapter) DeletePipeMessages(queueARN string, receiptHandles []string) error {
	url := arnToSQSQueueURL(queueARN)

	return a.backend.DeleteMessagesLocal(url, receiptHandles)
}

// pipesSFNStarterAdapter adapts the StepFunctions InMemoryBackend to the pipes.PipeStepFunctionsStarter interface.
type pipesSFNStarterAdapter struct {
	backend *sfnbackend.InMemoryBackend
}

func (a *pipesSFNStarterAdapter) StartExecution(stateMachineARN, name, input string) error {
	_, err := a.backend.StartExecution(stateMachineARN, name, input)

	return err
}

// pipesKinesisReaderAdapter adapts the Kinesis backend to the pipes.PipeKinesisReader
// source interface, mirroring kinesisReaderAdapter (Lambda's Kinesis ESM reader) but
// returning pipesbackend's own record type.
type pipesKinesisReaderAdapter struct {
	backend *kinesisbackend.InMemoryBackend
}

func (a *pipesKinesisReaderAdapter) GetShardIDs(streamName string) ([]string, error) {
	out, err := a.backend.DescribeStream(
		context.Background(),
		&kinesisbackend.DescribeStreamInput{StreamName: streamName},
	)
	if err != nil {
		return nil, err
	}

	ids := make([]string, len(out.Shards))
	for i, s := range out.Shards {
		ids[i] = s.ShardID
	}

	return ids, nil
}

func (a *pipesKinesisReaderAdapter) GetShardIterator(
	streamName, shardID, iteratorType, startingSeqNum string,
) (string, error) {
	out, err := a.backend.GetShardIterator(context.Background(), &kinesisbackend.GetShardIteratorInput{
		StreamName:             streamName,
		ShardID:                shardID,
		ShardIteratorType:      iteratorType,
		StartingSequenceNumber: startingSeqNum,
	})
	if err != nil {
		return "", err
	}

	return out.ShardIterator, nil
}

func (a *pipesKinesisReaderAdapter) GetRecords(
	iteratorToken string,
	limit int,
) ([]pipesbackend.KinesisRecord, string, error) {
	out, err := a.backend.GetRecords(context.Background(), &kinesisbackend.GetRecordsInput{
		ShardIterator: iteratorToken,
		Limit:         limit,
	})
	if err != nil {
		return nil, "", err
	}

	records := make([]pipesbackend.KinesisRecord, len(out.Records))
	for i, r := range out.Records {
		records[i] = pipesbackend.KinesisRecord{
			PartitionKey:   r.PartitionKey,
			SequenceNumber: r.SequenceNumber,
			Data:           r.Data,
			ArrivalTime:    r.ApproximateArrivalTimestamp,
		}
	}

	return records, out.NextShardIterator, nil
}

// pipesDDBStreamsReaderAdapter adapts the DynamoDB InMemoryDB to the
// pipes.PipeDynamoDBStreamsReader source interface, mirroring
// ddbStreamsReaderAdapter (Lambda's DynamoDB Streams ESM reader) but returning
// pipesbackend's own record type.
type pipesDDBStreamsReaderAdapter struct {
	backend *ddbbackend.InMemoryDB
}

func (a *pipesDDBStreamsReaderAdapter) DescribeStreamShards(streamARN string) ([]string, error) {
	out, err := a.backend.DescribeStream(
		context.Background(),
		&awsddbstreams.DescribeStreamInput{StreamArn: aws.String(streamARN)},
	)
	if err != nil {
		return nil, err
	}

	if out.StreamDescription == nil {
		return nil, nil
	}

	shardIDs := make([]string, 0, len(out.StreamDescription.Shards))
	for _, s := range out.StreamDescription.Shards {
		if s.ShardId != nil {
			shardIDs = append(shardIDs, *s.ShardId)
		}
	}

	return shardIDs, nil
}

func (a *pipesDDBStreamsReaderAdapter) GetStreamShardIterator(
	streamARN, shardID, iteratorType string,
) (string, error) {
	out, err := a.backend.GetShardIterator(
		context.Background(),
		&awsddbstreams.GetShardIteratorInput{
			StreamArn:         aws.String(streamARN),
			ShardId:           aws.String(shardID),
			ShardIteratorType: ddbstreamstypes.ShardIteratorType(iteratorType),
		},
	)
	if err != nil {
		return "", err
	}

	return aws.ToString(out.ShardIterator), nil
}

func (a *pipesDDBStreamsReaderAdapter) GetStreamRecords(
	iteratorToken string,
	limit int,
) ([]pipesbackend.DynamoDBStreamRecord, string, error) {
	// Clamp limit to a valid int32 range, mirroring ddbStreamsReaderAdapter.GetStreamRecords.
	const maxStreamRecordsLimit = math.MaxInt32

	lim := int32(math.MaxInt32)
	if limit > 0 && limit <= maxStreamRecordsLimit {
		lim = int32(limit)
	}

	out, err := a.backend.GetRecords(context.Background(), &awsddbstreams.GetRecordsInput{
		ShardIterator: aws.String(iteratorToken),
		Limit:         &lim,
	})
	if err != nil {
		return nil, "", err
	}

	records := make([]pipesbackend.DynamoDBStreamRecord, 0, len(out.Records))

	for _, r := range out.Records {
		rec := pipesbackend.DynamoDBStreamRecord{
			EventID:   aws.ToString(r.EventID),
			EventName: string(r.EventName),
		}

		populatePipesDDBStreamRecord(&rec, r.Dynamodb)
		records = append(records, rec)
	}

	return records, aws.ToString(out.NextShardIterator), nil
}

// populatePipesDDBStreamRecord fills in the DynamoDB-specific fields of a
// pipesbackend.DynamoDBStreamRecord from the SDK StreamRecord payload. Mirrors
// populateDDBStreamRecord (Lambda's equivalent) field-for-field.
func populatePipesDDBStreamRecord(
	rec *pipesbackend.DynamoDBStreamRecord,
	ddb *ddbstreamstypes.StreamRecord,
) {
	if ddb == nil {
		return
	}

	rec.SequenceNumber = aws.ToString(ddb.SequenceNumber)
	rec.StreamViewType = string(ddb.StreamViewType)

	if ddb.SizeBytes != nil {
		rec.SizeBytes = *ddb.SizeBytes
	}

	if ddb.ApproximateCreationDateTime != nil {
		rec.ApproximateCreationDateTime = float64(ddb.ApproximateCreationDateTime.Unix())
	}

	if ddb.NewImage != nil {
		rec.NewImage = sdkDDBStreamItemToWire(ddb.NewImage)
	}

	if ddb.OldImage != nil {
		rec.OldImage = sdkDDBStreamItemToWire(ddb.OldImage)
	}

	if ddb.Keys != nil {
		rec.Keys = sdkDDBStreamItemToWire(ddb.Keys)
	}
}

// pipesSNSPublisherAdapter adapts the SNS backend to the pipes.SNSPublisher
// target/DLQ interface.
type pipesSNSPublisherAdapter struct {
	backend *snsbackend.InMemoryBackend
}

func (a *pipesSNSPublisherAdapter) PublishMessage(_ context.Context, topicARN, message string) error {
	_, err := a.backend.Publish(topicARN, message, "", "", nil)

	return err
}

// pipesSQSSenderAdapter adapts the SQS backend to the pipes.SQSSender
// target/DLQ interface.
type pipesSQSSenderAdapter struct {
	backend *sqsbackend.InMemoryBackend
}

func (a *pipesSQSSenderAdapter) SendMessage(
	_ context.Context,
	queueARN, body, groupID, dedupID string,
) error {
	url := arnToSQSQueueURL(queueARN)
	_, err := a.backend.SendMessage(&sqsbackend.SendMessageInput{
		QueueURL:               url,
		MessageBody:            body,
		MessageGroupID:         groupID,
		MessageDeduplicationID: dedupID,
	})

	return err
}

// pipesKinesisPutterAdapter adapts the Kinesis backend to the
// pipes.PipeKinesisPutter target interface.
type pipesKinesisPutterAdapter struct {
	backend *kinesisbackend.InMemoryBackend
}

func (a *pipesKinesisPutterAdapter) PutRecord(ctx context.Context, streamARN, partitionKey string, data []byte) error {
	// Convert Kinesis stream ARN to stream name (last segment after '/').
	parts := strings.Split(streamARN, "/")
	streamName := parts[len(parts)-1]

	_, err := a.backend.PutRecord(ctx, &kinesisbackend.PutRecordInput{
		StreamName:   streamName,
		PartitionKey: partitionKey,
		Data:         data,
	})

	return err
}

// pipesEventBridgePutterAdapter adapts the EventBridge backend to the
// pipes.PipeEventBridgePutter target interface.
type pipesEventBridgePutterAdapter struct {
	backend *ebbackend.InMemoryBackend
}

func (a *pipesEventBridgePutterAdapter) PutEvents(
	ctx context.Context,
	eventBusARN string,
	events []map[string]any,
) error {
	// Convert event bus ARN to bus name (last segment after '/').
	parts := strings.Split(eventBusARN, "/")
	busName := parts[len(parts)-1]

	entries := make([]ebbackend.EventEntry, 0, len(events))

	for _, e := range events {
		entry := ebbackend.EventEntry{EventBusName: busName}
		if v, ok := e["Source"].(string); ok {
			entry.Source = v
		}
		if v, ok := e["DetailType"].(string); ok {
			entry.DetailType = v
		}
		if v, ok := e["Detail"].(string); ok {
			entry.Detail = v
		}

		entries = append(entries, entry)
	}

	_, err := a.backend.PutEvents(ctx, entries)

	return err
}

// pipesCloudWatchLogsPutterAdapter adapts the CloudWatch Logs backend to the
// pipes.PipeCloudWatchLogsPutter target interface.
type pipesCloudWatchLogsPutterAdapter struct {
	backend *cwlogsbackend.InMemoryBackend
}

func (a *pipesCloudWatchLogsPutterAdapter) PutLogEvents(
	ctx context.Context,
	logGroupARN, logStreamName string,
	messages []string,
) error {
	groupName := logGroupNameFromLogsARN(logGroupARN)
	now := time.Now().UnixMilli()

	events := make([]cwlogsbackend.InputLogEvent, len(messages))
	for i, msg := range messages {
		events[i] = cwlogsbackend.InputLogEvent{Message: msg, Timestamp: now}
	}

	_, err := a.backend.PutLogEvents(ctx, groupName, logStreamName, "", events)

	return err
}

// logGroupNameFromLogsARN converts a CloudWatch Logs log group identifier to a
// log group name. Log group identifiers may be ARNs
// (arn:...:log-group:<name>[:*]); in that case the name is extracted and any
// trailing ":*" wildcard suffix (the form other AWS services commonly specify
// a log group ARN target in) is stripped. Non-ARN identifiers are returned
// unchanged.
func logGroupNameFromLogsARN(id string) string {
	const marker = ":log-group:"

	idx := strings.LastIndex(id, marker)
	if idx < 0 {
		return id
	}

	return strings.TrimSuffix(id[idx+len(marker):], ":*")
}

// pipesFirehosePutterAdapter adapts the Firehose backend to the
// pipes.PipeFirehosePutter target interface.
type pipesFirehosePutterAdapter struct {
	backend *firehosebackend.InMemoryBackend
}

func (a *pipesFirehosePutterAdapter) PutRecord(ctx context.Context, deliveryStreamARN string, data []byte) error {
	// Convert Firehose delivery stream ARN to stream name (last segment after '/').
	parts := strings.Split(deliveryStreamARN, "/")
	streamName := parts[len(parts)-1]

	return a.backend.PutRecord(ctx, streamName, data)
}
