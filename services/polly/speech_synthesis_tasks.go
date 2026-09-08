package polly

import (
	"encoding/base64"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// snsTopicArnPattern matches a well-formed SNS topic ARN:
// arn:{partition}:sns:{region}:{12-digit account}:{name}.
var snsTopicArnPattern = regexp.MustCompile(
	`^arn:(aws|aws-cn|aws-us-gov|aws-iso|aws-iso-b):sns:[a-z0-9-]+:\d{12}:[A-Za-z0-9_-]{1,256}$`,
)

// StartSpeechSynthesisTask creates scheduled asynchronous task.
func (b *InMemoryBackend) StartSpeechSynthesisTask(
	options SynthesisOptions,
	outputBucket, outputKeyPrefix, topicArn string,
) (*SpeechSynthesisTask, error) {
	if outputBucket == "" {
		return nil, fmt.Errorf("%w: OutputS3BucketName is required", ErrValidation)
	}
	if !validS3BucketName(outputBucket) {
		return nil, fmt.Errorf(
			"%w: OutputS3BucketName %q is not a valid S3 bucket name", ErrInvalidS3Bucket, outputBucket,
		)
	}
	if !validS3KeyPrefix(outputKeyPrefix) {
		return nil, fmt.Errorf("%w: OutputS3KeyPrefix is not a valid S3 object key", ErrInvalidS3Key)
	}
	if !validSnsTopicArn(topicArn) {
		return nil, fmt.Errorf("%w: SnsTopicArn %q is not a valid SNS topic ARN", ErrInvalidSnsTopicArn, topicArn)
	}

	// AWS: StartSpeechSynthesisTask accepts up to 100,000 billed characters
	// (plain text) or 200,000 total characters (SSML, where markup is not
	// billed) -- see PARITY.md's "SpeechSynthesisTask API operations" quota.
	limit := maxTaskTextLen
	if options.TextType == textTypeSSML {
		limit = maxTaskSSMLLen
	}
	if len(options.Text) > limit {
		return nil, fmt.Errorf(
			"%w: text exceeds maximum length of %d characters", ErrTextLengthExceeded, limit,
		)
	}

	normal, err := b.validateOptions(options, true)
	if err != nil {
		return nil, err
	}

	id := uuid.NewString()
	task := &SpeechSynthesisTask{
		CreationTime: time.Now().UTC(),
		TaskID:       id,
		TaskStatus:   taskStatusScheduled,
		OutputURI: fmt.Sprintf(
			"s3://%s/%s%s.%s", outputBucket, outputKeyPrefix, id, taskExtension(normal.OutputFormat),
		),
		OutputS3BucketName: outputBucket,
		OutputS3KeyPrefix:  outputKeyPrefix,
		SNSTopicArn:        topicArn,
		Options:            normal,
	}

	b.mu.Lock("StartSpeechSynthesisTask")
	defer b.mu.Unlock()
	b.tasks.Put(task)

	return cloneTask(task), nil
}

// GetSpeechSynthesisTask retrieves task and advances simulated lifecycle.
func (b *InMemoryBackend) GetSpeechSynthesisTask(taskID string) (*SpeechSynthesisTask, error) {
	if _, err := uuid.Parse(taskID); err != nil {
		// AWS distinguishes a syntactically invalid TaskId (InvalidTaskIdException)
		// from a well-formed one that simply doesn't exist
		// (SynthesisTaskNotFoundException); task IDs are UUIDs (see the
		// uuid.NewString() call in StartSpeechSynthesisTask above).
		return nil, fmt.Errorf("%w: task id %q", ErrInvalidTaskID, taskID)
	}

	b.mu.Lock("GetSpeechSynthesisTask")
	defer b.mu.Unlock()

	task, ok := b.tasks.Get(taskID)
	if !ok {
		return nil, fmt.Errorf("%w: task %q", ErrTaskNotFound, taskID)
	}

	advanceTask(task)

	return cloneTask(task), nil
}

// validS3BucketName reports whether name follows AWS S3 bucket naming rules:
// 3-63 chars, lowercase letters/digits/dots/hyphens, must start and end with
// a letter or digit, no consecutive dots, and not formatted as an IP address.
func validS3BucketName(name string) bool {
	const minBucketLen, maxBucketLen = 3, 63
	if len(name) < minBucketLen || len(name) > maxBucketLen {
		return false
	}
	if net.ParseIP(name) != nil {
		return false
	}
	if !isAlphanumeric(rune(name[0])) || !isAlphanumeric(rune(name[len(name)-1])) {
		return false
	}

	prevDot := false
	for _, ch := range name {
		switch {
		case isAlphanumeric(ch):
			prevDot = false
		case ch == '.':
			if prevDot {
				return false
			}
			prevDot = true
		case ch == '-':
			prevDot = false
		default:
			return false
		}
	}

	return true
}

func isAlphanumeric(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
}

// validS3KeyPrefix reports whether prefix is safe to use as an S3 object key
// prefix: empty (the field is optional), or non-empty, at most 1024 bytes,
// and free of ASCII control characters, which AWS's object key naming
// guidelines flag as unsafe.
func validS3KeyPrefix(prefix string) bool {
	const maxS3KeyLen = 1024
	if prefix == "" {
		return true
	}
	if len(prefix) > maxS3KeyLen {
		return false
	}
	for _, ch := range prefix {
		if ch <= 0x1F || ch == 0x7F {
			return false
		}
	}

	return true
}

// validSnsTopicArn reports whether topicArn is empty (the field is
// optional) or a well-formed SNS topic ARN.
func validSnsTopicArn(topicArn string) bool {
	return topicArn == "" || snsTopicArnPattern.MatchString(topicArn)
}

// ListSpeechSynthesisTasks lists tasks and advances lifecycle consistently with AWS polling.
func (b *InMemoryBackend) ListSpeechSynthesisTasks(
	status, token string,
	maxResults int,
) ([]*SpeechSynthesisTask, string, error) {
	if status != "" && !slices.Contains(validTaskStatuses(), status) {
		return nil, "", fmt.Errorf("%w: invalid Status %q", ErrValidation, status)
	}

	b.mu.Lock("ListSpeechSynthesisTasks")
	defer b.mu.Unlock()

	// Table.Snapshot returns tasks ordered by TaskID ascending, matching the
	// previous collections.SortedKeys(b.tasks) traversal order exactly.
	tasks := b.tasks.Snapshot()

	offset, err := parseToken(token, len(tasks))
	if err != nil {
		return nil, "", err
	}
	if maxResults <= 0 || maxResults > maxTaskPageSize {
		maxResults = maxTaskPageSize
	}

	out := make([]*SpeechSynthesisTask, 0, len(tasks))
	for _, task := range tasks[offset:] {
		advanceTask(task)
		if status == "" || task.TaskStatus == status {
			out = append(out, cloneTask(task))
		}
		if len(out) == maxResults {
			return out, encodeToken(offset + len(out)), nil
		}
	}

	return out, "", nil
}

func cloneTask(task *SpeechSynthesisTask) *SpeechSynthesisTask {
	copyTask := *task
	copyTask.Options.LexiconNames = slices.Clone(task.Options.LexiconNames)
	copyTask.Options.SpeechMarkTypes = slices.Clone(task.Options.SpeechMarkTypes)

	return &copyTask
}

func advanceTask(task *SpeechSynthesisTask) {
	switch task.TaskStatus {
	case taskStatusScheduled:
		task.TaskStatus = taskStatusProgress
	case taskStatusProgress:
		if strings.Contains(strings.ToLower(task.Options.Text), failedTaskMarker) {
			task.TaskStatus = taskStatusFailed
			task.TaskStatusReason = "Synthetic synthesis failure requested by text marker"
		} else {
			task.TaskStatus = taskStatusCompleted
		}
	}
	task.polls++
}

func taskExtension(format string) string {
	if format == outputFormatOGG || format == outputFormatOggOpus {
		return "ogg"
	}

	return format
}

// encodeToken returns an opaque base64 cursor for pagination.
func encodeToken(n int) string {
	return base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(n)))
}

func parseToken(token string, total int) (int, error) {
	if token == "" {
		return 0, nil
	}
	raw := token
	if decoded, err := base64.StdEncoding.DecodeString(token); err == nil {
		raw = string(decoded)
	}
	var offset int
	if _, err := fmt.Sscanf(raw, "%d", &offset); err != nil || offset < 0 || offset > total {
		return 0, fmt.Errorf("%w: invalid NextToken", ErrInvalidNextToken)
	}

	return offset, nil
}

func validTaskStatuses() []string {
	return []string{taskStatusScheduled, taskStatusProgress, taskStatusCompleted, taskStatusFailed}
}
