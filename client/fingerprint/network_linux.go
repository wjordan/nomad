package fingerprint

import (
	"fmt"
	log "github.com/hashicorp/go-hclog"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// linkSpeedSys parses link speed in Mb/s from /sys.
func (f *NetworkFingerprint) linkSpeedSys(device string) int {
	path := fmt.Sprintf("/sys/class/net/%s/speed", device)

	// Read contents of the device/speed file
	content, err := ioutil.ReadFile(path)
	if err != nil {
		f.logger.Debug("unable to read link speed", "path", path)
		return 0
	}

	lines := strings.Split(string(content), "\n")
	mbs, err := strconv.Atoi(lines[0])
	if err != nil || mbs <= 0 {
		f.logger.Debug("unable to parse link speed", "path", path)
		return 0
	}

	return mbs
}

// linkSpeed returns link speed in Mb/s, or 0 when unable to determine it.
func (f *NetworkFingerprint) linkSpeed(device string) int {
	// Use LookPath to find the ethtool in the systems $PATH
	// If it's not found or otherwise errors, LookPath returns and empty string
	// and an error we can ignore for our purposes
	ethtoolPath, _ := exec.LookPath("ethtool")
	if ethtoolPath != "" {
		if speed := f.linkSpeedEthtool(ethtoolPath, device); speed > 0 {
			return speed
		}
	}

	// Fall back on checking a system file for link speed.
	return f.linkSpeedSys(device)
}

// linkSpeedEthtool determines link speed in Mb/s with 'ethtool'.
func (f *NetworkFingerprint) linkSpeedEthtool(path, device string) int {
	outBytes, err := exec.Command(path, device).Output()
	if err != nil {
		f.logger.Warn("error calling ethtool", "error", err, "path", path, "device", device)
		return 0
	}

	output := strings.TrimSpace(string(outBytes))
	re := regexp.MustCompile("Speed: [0-9]+[a-zA-Z]+/s")
	m := re.FindString(output)
	if m == "" {
		// no matches found, output may be in a different format
		f.logger.Warn("unable to parse speed", "path", path, "device", device)
		return 0
	}

	// Split and trim the Mb/s unit from the string output
	args := strings.Split(m, ": ")
	raw := strings.TrimSuffix(args[1], "Mb/s")

	// convert to Mb/s
	mbs, err := strconv.Atoi(raw)
	if err != nil || mbs <= 0 {
		f.logger.Warn("unable to parse Mb/s", "path", path, "device", device)
		return 0
	}

	return mbs
}

type LinuxNetInterface struct {
	net.Interface
	addrs []net.Addr
}

// NetInterfaces returns all net.Interfaces along with their associated []net.Addr.
// This Linux optimization avoids a separate Netlink dump of addresses for each individual interface,
// which is prohibitively slow on servers with large numbers of interfaces.
func NetInterfaces() ([]LinuxNetInterface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	netInterfaces := make([]LinuxNetInterface, 0)
	for _, ifi := range interfaces {
		netInterfaces = append(netInterfaces, LinuxNetInterface{ifi, make([]net.Addr, 0)})
	}
	ifMap := make(map[int]LinuxNetInterface)
	for _, ifi := range netInterfaces {
		ifMap[ifi.Index] = ifi
	}

	tab, err := syscall.NetlinkRIB(syscall.RTM_GETADDR, syscall.AF_UNSPEC)
	if err != nil {
		return nil, os.NewSyscallError("NetlinkRIB", err)
	}
	msgs, err := syscall.ParseNetlinkMessage(tab)
	if err != nil {
		return nil, os.NewSyscallError("ParseNetLinkMessage", err)
	}

	for _, m := range msgs {
		if m.Header.Type == syscall.RTM_NEWADDR {
			ifam := (*syscall.IfAddrmsg)(unsafe.Pointer(&m.Data[0]))
			attrs, err := syscall.ParseNetlinkRouteAttr(&m)
			if err != nil {
				return nil, os.NewSyscallError("ParseNetLinkRouteAttr", err)
			}
			if ifi, ok := ifMap[int(ifam.Index)]; ok {
				ifi.addrs = append(ifi.addrs, newAddr(ifam, attrs))
			}
		}
	}

	return netInterfaces, err
}

// Vendored unexported function:
// https://github.com/golang/go/blob/8bcc490667d4dd44c633c536dd463bbec0a3838f/src/net/interface_linux.go#L178-L203
func newAddr(ifam *syscall.IfAddrmsg, attrs []syscall.NetlinkRouteAttr) net.Addr {
	var ipPointToPoint bool
	for _, a := range attrs {
		if a.Attr.Type == syscall.IFA_LOCAL {
			ipPointToPoint = true
			break
		}
	}
	for _, a := range attrs {
		if ipPointToPoint && a.Attr.Type == syscall.IFA_ADDRESS {
			continue
		}
		switch ifam.Family {
		case syscall.AF_INET:
			return &net.IPNet{IP: net.IPv4(a.Value[0], a.Value[1], a.Value[2], a.Value[3]), Mask: net.CIDRMask(int(ifam.Prefixlen), 8*net.IPv4len)}
		case syscall.AF_INET6:
			ifa := &net.IPNet{IP: make(net.IP, net.IPv6len), Mask: net.CIDRMask(int(ifam.Prefixlen), 8*net.IPv6len)}
			copy(ifa.IP, a.Value[:])
			return ifa
		}
	}
	return nil
}

// NewNetworkFingerprint returns a new NetworkFingerprinter with the given
// logger
func NewNetworkFingerprint(logger log.Logger) Fingerprint {
	f := &NetworkFingerprint{logger: logger.Named("network"), interfaceDetector: &LinuxNetworkInterfaceDetector{}}
	return f
}

// LinuxNetworkInterfaceDetector implements the interface detector with Linux optimizations.
type LinuxNetworkInterfaceDetector struct {
	interfaces map[int]LinuxNetInterface
}

func (b *LinuxNetworkInterfaceDetector) Interfaces() ([]net.Interface, error) {
	interfaces, err := NetInterfaces()
	if err != nil {
		return nil, err
	}
	netInterfaces := make([]net.Interface, 0)
	ifMap := make(map[int]LinuxNetInterface)
	for _, i := range interfaces {
		ifMap[i.Index] = i
		netInterfaces = append(netInterfaces, i.Interface)
	}
	b.interfaces = ifMap
	return netInterfaces, nil
}

func (b *LinuxNetworkInterfaceDetector) InterfaceByName(name string) (*net.Interface, error) {
	return net.InterfaceByName(name)
}

func (b *LinuxNetworkInterfaceDetector) Addrs(intf *net.Interface) ([]net.Addr, error) {
	if b.interfaces == nil {
		if _, err := b.Interfaces(); err != nil {
			return nil, err
		}
	}
	if i, ok := b.interfaces[intf.Index]; ok {
		return i.addrs, nil
	} else {
		return intf.Addrs()
	}
}
