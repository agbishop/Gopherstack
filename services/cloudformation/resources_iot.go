package cloudformation

import (
	"fmt"

	iotbackend "github.com/blackbirdworks/gopherstack/services/iot"
)

// ---- IoT ----

func (rc *ResourceCreator) createIoTThing(
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, error) {
	if rc.backends.IoT == nil {
		return logicalID + "-stub", nil
	}

	thingName := strProp(props, "ThingName", params, physicalIDs)
	if thingName == "" {
		thingName = logicalID
	}

	out, err := rc.backends.IoT.Backend.CreateThing(&iotbackend.CreateThingInput{
		ThingName: thingName,
	})
	if err != nil {
		return "", fmt.Errorf("create IoT thing %s: %w", thingName, err)
	}

	return out.ThingARN, nil
}

func (rc *ResourceCreator) deleteIoTThing(arn string) error {
	if rc.backends.IoT == nil {
		return nil
	}

	name := resourceNameFromARN(arn)

	return rc.backends.IoT.Backend.DeleteThing(name, 0)
}

func (rc *ResourceCreator) createIoTTopicRule(
	logicalID string,
	props map[string]any,
	params, physicalIDs map[string]string,
) (string, error) {
	if rc.backends.IoT == nil {
		return logicalID + "-stub", nil
	}

	ruleName := strProp(props, "RuleName", params, physicalIDs)
	if ruleName == "" {
		ruleName = logicalID
	}

	var payload *iotbackend.TopicRulePayload
	if tp, ok := props["TopicRulePayload"].(map[string]any); ok {
		payload = &iotbackend.TopicRulePayload{
			SQL:         resolve(tp["SQL"], params, physicalIDs),
			Description: resolve(tp["Description"], params, physicalIDs),
		}
	}

	err := rc.backends.IoT.Backend.CreateTopicRule(&iotbackend.CreateTopicRuleInput{
		RuleName:         ruleName,
		TopicRulePayload: payload,
	})
	if err != nil {
		return "", fmt.Errorf("create IoT topic rule %s: %w", ruleName, err)
	}

	return "arn:aws:iot:" + rc.backends.Region + ":" + rc.backends.AccountID + ":rule/" + ruleName, nil
}

func (rc *ResourceCreator) deleteIoTTopicRule(arn string) error {
	if rc.backends.IoT == nil {
		return nil
	}

	name := resourceNameFromARN(arn)

	return rc.backends.IoT.Backend.DeleteTopicRule(name)
}
