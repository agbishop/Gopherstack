package cloudwatch

import (
	"net/http"
	"time"

	"github.com/aws/smithy-go/encoding/cbor"
	"github.com/labstack/echo/v5"
)

func (h *Handler) cborDescribeAlarmHistory(input cbor.Map, c *echo.Context) error {
	alarmName := cborStr(input, "AlarmName")
	alarmTypes := cborStrList(input, "AlarmTypes")
	historyItemType := cborStr(input, "HistoryItemType")
	nextToken := cborStr(input, "NextToken")
	scanBy := cborStr(input, "ScanBy")
	maxRecords := int(cborInt32(input, "MaxRecords"))

	// Treat zero-value times as unset (cborTime returns now when key is missing).
	var sd, ed time.Time
	if _, hasStart := input["StartDate"]; hasStart {
		sd = cborTime(input, "StartDate")
	}
	if _, hasEnd := input["EndDate"]; hasEnd {
		ed = cborTime(input, "EndDate")
	}

	p, err := h.Backend.DescribeAlarmHistory(
		alarmName,
		alarmTypes,
		historyItemType,
		nextToken,
		scanBy,
		sd,
		ed,
		maxRecords,
	)
	if err != nil {
		return h.cborError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	histList := make(cbor.List, 0, len(p.Data))
	for _, item := range p.Data {
		m := cbor.Map{
			keyAlarmName:      cbor.String(item.AlarmName),
			"HistoryItemType": cbor.String(item.HistoryItemType),
			"HistorySummary":  cbor.String(item.HistorySummary),
			"HistoryData":     cbor.String(item.HistoryData),
			"Timestamp":       cborFromTime(item.Timestamp),
		}
		if item.AlarmType != "" {
			m["AlarmType"] = cbor.String(item.AlarmType)
		}
		histList = append(histList, m)
	}

	resp := cbor.Map{"AlarmHistoryItems": histList}
	if p.Next != "" {
		resp["NextToken"] = cbor.String(p.Next)
	}

	return writeCBOR(c, resp)
}
