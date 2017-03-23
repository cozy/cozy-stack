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

func (h *hub) getTopic(prefix, t string) *topic {
	h.RLock()
	defer h.RUnlock()
	return h.topics[topicKey(prefix, t)]
}

func (h *hub) getTopicOrCreate(prefix, t string) *topic {
	h.Lock()
	defer h.Unlock()
	key := topicKey(prefix, t)
	it, exists := h.topics[key]
	if !exists {
		it = makeTopic(h, key)
		h.topics[key] = it
	}
	return it
}

func (h *hub) Subscribe(t string) EventChannel {
	gt := h.getTopicOrCreate(global, t)
	sub := makeSub([]*topic{gt})
	gt.subscribe <- sub
	return sub
}

func (h *hub) remove(t *topic) {
	h.Lock()
	defer h.Unlock()
	delete(h.topics, t.key)
}

type instancehub struct {
	prefix  string
	mainHub *hub
}

func (ih *instancehub) Publish(e *Event) {
	it := ih.mainHub.getTopic(ih.prefix, e.Doc.DocType())
	if it != nil {
		it.broadcast <- e
	}
	e.Instance = ih.prefix
	gt := ih.mainHub.getTopic(global, e.Doc.DocType())
	if gt != nil {
		gt.broadcast <- e
	}
}

func (ih *instancehub) Subscribe(t string) EventChannel {
	it := ih.mainHub.getTopicOrCreate(ih.prefix, t)
	sub := makeSub([]*topic{it})
	it.subscribe <- sub
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
		prefix:  domain,
		mainHub: mainHub,
	}
}
