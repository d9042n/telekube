package incident_test

import (
	"testing"
	"time"

	"github.com/d9042n/telekube/internal/module/incident"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Timeline.Sort ────────────────────────────────────────────────────────────

func TestTimeline_Sort_OrdersChronologically(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tl := &incident.Timeline{}
	tl.Append(incident.TimelineEvent{Timestamp: now.Add(2 * time.Minute), Summary: "third"})
	tl.Append(incident.TimelineEvent{Timestamp: now.Add(1 * time.Minute), Summary: "second"})
	tl.Append(incident.TimelineEvent{Timestamp: now, Summary: "first"})

	tl.Sort()

	require.Len(t, tl.Events, 3)
	assert.Equal(t, "first", tl.Events[0].Summary)
	assert.Equal(t, "second", tl.Events[1].Summary)
	assert.Equal(t, "third", tl.Events[2].Summary)
}

func TestTimeline_Sort_Empty_DoesNotPanic(t *testing.T) {
	t.Parallel()

	tl := &incident.Timeline{}
	assert.NotPanics(t, tl.Sort)
	assert.Empty(t, tl.Events)
}

func TestTimeline_Sort_SingleEvent(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tl := &incident.Timeline{}
	tl.Append(incident.TimelineEvent{Timestamp: now, Summary: "only"})
	tl.Sort()
	require.Len(t, tl.Events, 1)
}

// ─── Timeline.Append ──────────────────────────────────────────────────────────

func TestTimeline_Append_IncreasesLength(t *testing.T) {
	t.Parallel()

	tl := &incident.Timeline{}
	for i := 0; i < 5; i++ {
		tl.Append(incident.TimelineEvent{
			Timestamp: time.Now(),
			Summary:   "event",
		})
	}
	assert.Len(t, tl.Events, 5)
}

func TestTimeline_Append_PreservesFields(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tl := &incident.Timeline{}
	tl.Append(incident.TimelineEvent{
		Timestamp: now,
		Emoji:     "🔴",
		Category:  "k8s",
		Summary:   "nginx — BackOff (restarting)",
	})

	require.Len(t, tl.Events, 1)
	e := tl.Events[0]
	assert.Equal(t, now, e.Timestamp)
	assert.Equal(t, "🔴", e.Emoji)
	assert.Equal(t, "k8s", e.Category)
	assert.Equal(t, "nginx — BackOff (restarting)", e.Summary)
}

// ─── Timeline metadata ────────────────────────────────────────────────────────

func TestTimeline_Metadata_Fields(t *testing.T) {
	t.Parallel()

	from := time.Now().Add(-30 * time.Minute)
	to := time.Now()

	tl := &incident.Timeline{
		Namespace: "production",
		Cluster:   "prod-1",
		From:      from,
		To:        to,
	}

	assert.Equal(t, "production", tl.Namespace)
	assert.Equal(t, "prod-1", tl.Cluster)
	assert.Equal(t, from, tl.From)
	assert.Equal(t, to, tl.To)
}

func TestTimeline_Duration_Calculation(t *testing.T) {
	t.Parallel()

	from := time.Now().Add(-1 * time.Hour)
	to := time.Now()

	tl := &incident.Timeline{From: from, To: to}

	duration := tl.To.Sub(tl.From)
	assert.True(t, duration >= 59*time.Minute && duration <= 61*time.Minute,
		"duration should be ~1 hour, got %v", duration)
}

// ─── Sort stability — events with same timestamp ──────────────────────────────

func TestTimeline_Sort_SameTimestamp_DoesNotPanic(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tl := &incident.Timeline{}
	for i := 0; i < 10; i++ {
		tl.Append(incident.TimelineEvent{
			Timestamp: now, // all same
			Summary:   "simultaneous",
		})
	}

	assert.NotPanics(t, tl.Sort)
	assert.Len(t, tl.Events, 10)
}

// ─── Edge: many events beyond display limit ───────────────────────────────────

func TestTimeline_ManyEvents_AllAppended(t *testing.T) {
	t.Parallel()

	tl := &incident.Timeline{}
	now := time.Now()
	for i := 0; i < 50; i++ {
		tl.Append(incident.TimelineEvent{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Summary:   "event",
		})
	}

	assert.Len(t, tl.Events, 50, "all 50 events must be appended")

	tl.Sort()
	// After sort, events should still be 50.
	assert.Len(t, tl.Events, 50)
	// And in order: first event has the smallest timestamp.
	assert.True(t, tl.Events[0].Timestamp.Before(tl.Events[49].Timestamp))
}

// ─── Category filtering (K8s vs User) ────────────────────────────────────────

func TestTimeline_Categories_CorrectlyTagged(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tl := &incident.Timeline{}
	tl.Append(incident.TimelineEvent{Timestamp: now, Category: "k8s", Summary: "k8s event"})
	tl.Append(incident.TimelineEvent{Timestamp: now, Category: "user", Summary: "user action"})

	k8sCount, userCount := 0, 0
	for _, e := range tl.Events {
		if e.Category == "k8s" {
			k8sCount++
		}
		if e.Category == "user" {
			userCount++
		}
	}

	assert.Equal(t, 1, k8sCount)
	assert.Equal(t, 1, userCount)
}
