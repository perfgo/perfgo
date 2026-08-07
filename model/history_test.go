package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerfRecordHistorySupportsMultipleEvents(t *testing.T) {
	record := PerfRecord{Events: []string{"cycles", "instructions"}, Count: 1000}

	data, err := json.Marshal(record)
	require.NoError(t, err)
	assert.JSONEq(t, `{"events":["cycles","instructions"],"count":1000}`, string(data))

	var decoded PerfRecord
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, record, decoded)
}

func TestPerfRecordHistoryReadsLegacyEvent(t *testing.T) {
	var record PerfRecord
	require.NoError(t, json.Unmarshal([]byte(`{"event":"cycles","count":1000}`), &record))

	assert.Equal(t, "cycles", record.Event)
	assert.Empty(t, record.Events)
	assert.Equal(t, 1000, record.Count)
}
