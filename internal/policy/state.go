package policy

import (
	"sync"

	hsarv1 "github.com/hsar-org/hsar/gen/go/hsar/v1"
)

// ConversationState is the per-conversation FSM snapshot.
type ConversationState struct {
	StabilityState    hsarv1.StabilityState
	CooldownRemaining int
	MatchedRuleIndex  int
	LastSignalValue   float32
}

// StateStore holds FSM state keyed by tenant:conversation.
type StateStore struct {
	mu    sync.Mutex
	store map[string]*sync.Mutex
	data  map[string]ConversationState
}

func NewStateStore() *StateStore {
	return &StateStore{
		store: map[string]*sync.Mutex{},
		data:  map[string]ConversationState{},
	}
}

func (s *StateStore) key(tenantID, conversationID string) string {
	return tenantID + ":" + conversationID
}

func (s *StateStore) lockFor(key string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.store[key]; ok {
		return m
	}
	m := &sync.Mutex{}
	s.store[key] = m
	if _, ok := s.data[key]; !ok {
		s.data[key] = ConversationState{StabilityState: hsarv1.StabilityState_STATE_NORMAL}
	}
	return m
}

// Get returns a copy of state for key.
func (s *StateStore) Get(tenantID, conversationID string) ConversationState {
	key := s.key(tenantID, conversationID)
	m := s.lockFor(key)
	m.Lock()
	defer m.Unlock()
	return s.data[key]
}

// Update runs fn with exclusive access to conversation state.
func (s *StateStore) Update(tenantID, conversationID string, fn func(*ConversationState)) {
	key := s.key(tenantID, conversationID)
	m := s.lockFor(key)
	m.Lock()
	defer m.Unlock()
	st := s.data[key]
	fn(&st)
	s.data[key] = st
}

// transitionFSM applies hysteresis + cooldown rules.
func transitionFSM(st *ConversationState, signalValue float32, rule PolicyRule, cooldownRequests int) {
	st.LastSignalValue = signalValue

	switch st.StabilityState {
	case hsarv1.StabilityState_STATE_UNSPECIFIED, hsarv1.StabilityState_STATE_NORMAL:
		if signalValue >= rule.EnterThreshold {
			st.StabilityState = hsarv1.StabilityState_STATE_HYSTERESIS_ENTRY
			st.StabilityState = hsarv1.StabilityState_STATE_ACTIVE
		}
	case hsarv1.StabilityState_STATE_HYSTERESIS_ENTRY, hsarv1.StabilityState_STATE_ACTIVE:
		if signalValue < rule.ExitThreshold {
			st.StabilityState = hsarv1.StabilityState_STATE_COOLDOWN
			st.CooldownRemaining = cooldownRequests
		}
	case hsarv1.StabilityState_STATE_COOLDOWN:
		if signalValue >= rule.ExitThreshold {
			st.StabilityState = hsarv1.StabilityState_STATE_ACTIVE
			st.CooldownRemaining = 0
			return
		}
		if st.CooldownRemaining > 0 {
			st.CooldownRemaining--
			st.StabilityState = hsarv1.StabilityState_STATE_ACTIVE
			return
		}
		st.StabilityState = hsarv1.StabilityState_STATE_NORMAL
	}
}