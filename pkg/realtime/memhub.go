package realtime

import "sync"

const global = "io.cozy.realtime.global.instance"

type hub struct {
	sync.RWMutex
	// topics should only be manipulated by the hub loop
	// it is a Map(instance + type -> Topic)
	topics map[string]*topic
}

func (h *hub) Publish(e *Event) {
	panic("Wrong usage : you should not publish on central hub.")
}

func (h *hub) getTopic(instance, t string, createIfNone bool) *topic {
	if !createIfNone {
		h.RLock()
		defer h.RUnlock()
	} else {
		h.Lock()
		defer h.Unlock()
	}
	key := topicKey(instance, t)
	it, exists := h.topics[key]
	if !exists && createIfNone {
		h.topics[key] = makeTopic(h, key)
		it = h.topics[key]
	}
	return it
}

func (h *hub) Subscribe(t string) EventChannel {
	gt := h.getTopic(global, t, true)
	sub := makeSub([]*topic{gt})
	go func() { gt.subscribe <- sub }()
	return sub
}

func (h *hub) remove(t *topic) {
	h.Lock()
	defer h.Unlock()
	delete(h.topics, t.key)
}

type instancehub struct {
	instance string
	mainHub  *hub
}

func (ih *instancehub) Publish(e *Event) {
	it := ih.mainHub.getTopic(ih.instance, e.DocType, false)
	if it != nil {
		it.publish(e)
	}
	e.Instance = ih.instance
	gt := ih.mainHub.getTopic(global, e.DocType, false)
	if gt != nil {
		gt.publish(e)
	}
}

func (ih *instancehub) Subscribe(t string) EventChannel {
	it := ih.mainHub.getTopic(ih.instance, t, true)
	sub := makeSub([]*topic{it})
	go func() { it.subscribe <- sub }()

	return sub
}

var mainHub *hub

func init() {
	mainHub = &hub{
		topics: make(map[string]*topic),
	}
}

// MainHub returns the central memory hub
func MainHub() Hub {
	return mainHub
}

// InstanceHub returns a memory hub for an Instance
func InstanceHub(domain string) Hub {
	return &instancehub{
		instance: domain,
		mainHub:  mainHub,
	}
}
