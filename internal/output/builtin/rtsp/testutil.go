package rtsp

func ResetListenerRegistryForTest() {
	registryMu.Lock()
	defer registryMu.Unlock()
	for _, sl := range listenerRegistry {
		sl.cancel()
		_ = sl.listener.Close()
	}
	listenerRegistry = make(map[string]*sharedListener)
}
