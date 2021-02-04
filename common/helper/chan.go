package helper

func AsyncSendErr(ch chan error, err error) {
	if ch == nil {
		return
	}
	select {
	case ch <- err:
	default:
	}
}

// asyncNotify is used to signal a waiting goroutine
func AsyncNotify(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}
