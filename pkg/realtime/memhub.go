package realtime

import "sync"

type hub struct {
	sync.Mutex

	// topics should only be manipulated by the hub loop
	// it is a Map(instance + type -> Topic)
	instanceTopics map[string]*topic

	// Map(domain -> Topic)
	globalTopics map[string]*topic
}

func (h *hub) Publish(e *Event) {
	panic("Wrong usage : you should not publish on central hub.")
}

func (h *hub) Subscribe(t string) EventChannel {
	h.Lock()
	defer h.Unlock()
	gt, ok := h.globalTopics[t]
	if !ok {
		h.globalTopics[t] = makeTopic(h, t)
		gt = h.globalTopics[t]
	}
	sub := makeSub([]*topic{gt})
	go func() { gt.subscribe <- sub }()

	return sub
}

func (h *hub) remove(t *topic) {
	h.Lock()
	defer h.Unlock()
	delete(h.globalTopics, t.key)
	delete(h.instanceTopics, t.key)
}

type instancehub struct {
	instance string
	mainHub  *hub
}

func (ih *instancehub) Publish(e *Event) {
	ih.mainHub.Lock()
	defer ih.mainHub.Unlock()
	it, ok := ih.mainHub.instanceTopics[topicKey(ih.instance, e.DocType)]
	if ok {
		it.publish(e)
	}
	e.Instance = ih.instance
	gt, ok := ih.mainHub.globalTopics[e.DocType]
	if ok {
		gt.publish(e)
	}
}

func (ih *instancehub) Subscribe(t string) EventChannel {
	ih.mainHub.Lock()
	defer ih.mainHub.Unlock()
	key := topicKey(ih.instance, t)
	it, ok := ih.mainHub.instanceTopics[key]
	if !ok {
		ih.mainHub.instanceTopics[key] = makeTopic(ih.mainHub, key)
		it = ih.mainHub.instanceTopics[key]
	}
	sub := makeSub([]*topic{it})
	go func() { it.subscribe <- sub }()

	return sub
}

var mainHub *hub

func init() {
	mainHub = &hub{
		instanceTopics: make(map[string]*topic),
		globalTopics:   make(map[string]*topic),
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
