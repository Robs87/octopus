package balancer

import (
	"testing"

	"github.com/bestruirui/octopus/internal/model"
)

func TestIteratorExposesStickyChannelKeyID(t *testing.T) {
	const apiKeyID = 901
	const requestModel = "sticky-model"
	SetSticky(apiKeyID, requestModel, 7, 72)
	defer globalSession.Delete(sessionKey(apiKeyID, requestModel))

	it := NewIterator(model.Group{
		Mode:            model.GroupModeFailover,
		SessionKeepTime: 60,
		Items:           []model.GroupItem{{ChannelID: 7, ModelName: requestModel}},
	}, apiKeyID, requestModel)
	if !it.Next() {
		t.Fatal("iterator has no candidate")
	}
	if !it.IsSticky() {
		t.Fatal("first candidate is not sticky")
	}
	if got := it.StickyChannelKeyID(); got != 72 {
		t.Fatalf("StickyChannelKeyID() = %d, want 72", got)
	}
}
