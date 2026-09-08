package cloudwatch

import (
	"encoding/xml"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

func (h *Handler) handleDescribeAlarmHistory(form url.Values, c *echo.Context) error {
	alarmName := form.Get("AlarmName")
	alarmTypes := parseMemberList(form, "AlarmTypes.")
	historyItemType := form.Get("HistoryItemType")
	nextToken := form.Get("NextToken")
	scanBy := form.Get("ScanBy")
	maxRecords, _ := strconv.Atoi(form.Get("MaxRecords"))

	var startDate, endDate time.Time
	if s := form.Get("StartDate"); s != "" {
		startDate, _ = time.Parse(time.RFC3339, s)
	}
	if e := form.Get("EndDate"); e != "" {
		endDate, _ = time.Parse(time.RFC3339, e)
	}

	p, err := h.Backend.DescribeAlarmHistory(
		alarmName,
		alarmTypes,
		historyItemType,
		nextToken,
		scanBy,
		startDate,
		endDate,
		maxRecords,
	)
	if err != nil {
		return h.xmlError(c, http.StatusInternalServerError, "InternalFailure", err.Error())
	}

	type historyItemXML struct {
		AlarmName       string `xml:"AlarmName"`
		AlarmType       string `xml:"AlarmType,omitempty"`
		HistoryItemType string `xml:"HistoryItemType"`
		HistorySummary  string `xml:"HistorySummary"`
		HistoryData     string `xml:"HistoryData,omitempty"`
		Timestamp       string `xml:"Timestamp"`
	}
	members := make([]historyItemXML, 0, len(p.Data))
	for _, item := range p.Data {
		members = append(members, historyItemXML{
			AlarmName:       item.AlarmName,
			AlarmType:       item.AlarmType,
			HistoryItemType: item.HistoryItemType,
			HistorySummary:  item.HistorySummary,
			HistoryData:     item.HistoryData,
			Timestamp:       item.Timestamp.UTC().Format(time.RFC3339),
		})
	}

	type descResult struct {
		NextToken         string           `xml:"NextToken,omitempty"`
		AlarmHistoryItems []historyItemXML `xml:"AlarmHistoryItems>member"`
	}
	type response struct {
		XMLName   xml.Name   `xml:"DescribeAlarmHistoryResponse"`
		Xmlns     string     `xml:"xmlns,attr"`
		RequestID string     `xml:"ResponseMetadata>RequestId"`
		Result    descResult `xml:"DescribeAlarmHistoryResult"`
	}

	return writeXML(c, response{
		Xmlns:     cloudwatchNS,
		Result:    descResult{AlarmHistoryItems: members, NextToken: p.Next},
		RequestID: uuid.New().String(),
	})
}
