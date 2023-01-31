// +build !linux,!windows

package fingerprint

// linkSpeed returns the default link speed
func (f *NetworkFingerprint) linkSpeed(device string) int {
	return 0
}

// NewNetworkFingerprint returns a new NetworkFingerprinter with the given
// logger
func NewNetworkFingerprint(logger log.Logger) Fingerprint {
	f := &NetworkFingerprint{logger: logger.Named("network"), interfaceDetector: &DefaultNetworkInterfaceDetector{}}
	return f
}
