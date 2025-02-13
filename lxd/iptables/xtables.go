package iptables

import (
	"encoding/hex"
	"fmt"
	"net"
	"strings"

	deviceConfig "github.com/lxc/lxd/lxd/device/config"
	firewallConsts "github.com/lxc/lxd/lxd/firewall/consts"
	"github.com/lxc/lxd/lxd/util"
	"github.com/lxc/lxd/shared"
)

// XTables is an implmentation of LXD firewall using {ip, ip6, eb}tables
type XTables struct{}

// Lower-level Functions

// NetworkClear removes network rules.
func (xt XTables) NetworkClear(family firewallConsts.Family, table firewallConsts.Table, comment string) error {
	return NetworkClear(fmt.Sprintf("%s", family), comment, fmt.Sprintf("%s", table))
}

// InstanceClear removes rules all rules for the given instance.
func (xt XTables) InstanceClear(family firewallConsts.Family, table firewallConsts.Table, comment string) error {
	return ContainerClear(fmt.Sprintf("%s", family), comment, fmt.Sprintf("%s", table))
}

// VerifyIPv6Module checks to see if the ipv6 kernel module is present.
func (xt XTables) VerifyIPv6Module() error {
	// Check br_netfilter is loaded and enabled for IPv6.
	sysctlPath := "net/bridge/bridge-nf-call-ip6tables"
	sysctlVal, err := util.SysctlGet(sysctlPath)
	if err != nil {
		return fmt.Errorf("Error reading net sysctl %s: %v", sysctlPath, err)
	}

	if sysctlVal != "1\n" {
		return fmt.Errorf("security.ipv6_filtering requires br_netfilter and sysctl net.bridge.bridge-nf-call-ip6tables=1")
	}

	return nil
}

// Proxy Functions

// InstanceProxySetupNAT creates a default NAT setup.
func (xt XTables) InstanceProxySetupNAT(family firewallConsts.Family, connType, address, port string, destAddr net.IP, destPort string, comment string) error {
	toDest := fmt.Sprintf("%s:%s", destAddr, destPort)
	if family == "ipv6" {
		toDest = fmt.Sprintf("[%s]:%s", destAddr, destPort)
	}

	// outbound <-> container
	err := ContainerPrepend(fmt.Sprintf("%s", family), comment, "nat", "PREROUTING", "-p", connType, "--destination", address, "--dport", port, "-j", "DNAT", "--to-destination", toDest)
	if err != nil {
		return err
	}

	// host <-> container
	err = ContainerPrepend(fmt.Sprintf("%s", family), comment, "nat", "OUTPUT", "-p", connType, "--destination", address, "--dport", port, "-j", "DNAT", "--to-destination", toDest)
	if err != nil {
		return err
	}

	return nil
}

// NIC Bridged Functions

// InstanceNicBridgedRemoveFilters removes any non-standard rules from the nic instance.
func (xt XTables) InstanceNicBridgedRemoveFilters(m deviceConfig.Device, ipv4 net.IP, ipv6 net.IP) error {
	// Get a current list of rules active on the host.
	out, err := shared.RunCommand("ebtables", "--concurrent", "-L", "--Lmac2", "--Lx")
	if err != nil {
		return fmt.Errorf("Failed to remove network filters for %s: %v", m["name"], err)
	}

	// Get a list of rules that we would have applied on instance start.
	rules := generateFilterEbtablesRules(m, ipv4, ipv6)

	errs := []error{}
	// Iterate through each active rule on the host and try and match it to one the LXD rules.
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		fields := strings.Fields(line)
		fieldsLen := len(fields)

		for _, rule := range rules {
			// Rule doesn't match if the field lenths aren't the same, move on.
			if len(rule) != fieldsLen {
				continue
			}

			// Check whether active rule matches one of our rules to delete.
			if !matchEbtablesRule(fields, rule, true) {
				continue
			}

			// If we get this far, then the current host rule matches one of our LXD
			// rules, so we should run the modified command to delete it.
			_, err = shared.RunCommand(fields[0], append([]string{"--concurrent"}, fields[1:]...)...)
			if err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("Failed to remove network filters rule for %s: %v", m["name"], errs)
	}

	return nil
}

// InstanceNicBridgedSetFilters sets the nic rules to standard filtering.
func (xt XTables) InstanceNicBridgedSetFilters(m deviceConfig.Device, ipv4 net.IP, ipv6 net.IP, comment string) error {
	rules := generateFilterEbtablesRules(m, ipv4, ipv6)
	for _, rule := range rules {
		_, err := shared.RunCommand(rule[0], append([]string{"--concurrent"}, rule[1:]...)...)
		if err != nil {
			return err
		}
	}

	rules, err := generateFilterIptablesRules(m, ipv6)
	if err != nil {
		return err
	}

	for _, rule := range rules {
		err = ContainerPrepend(rule[0], fmt.Sprintf("%s - %s_filtering", comment, rule[0]), "filter", rule[1], rule[2:]...)
		if err != nil {
			return err
		}
	}

	return nil
}

// Network Functions

// NetworkSetupAllowForwarding allows forwarding dependent on boolean argument
func (xt XTables) NetworkSetupAllowForwarding(family firewallConsts.Family, name string, actionType firewallConsts.Action) error {
	forwardType := "DROP"
	if actionType == firewallConsts.ActionAccept {
		forwardType = "ACCEPT"
	} else if actionType == firewallConsts.ActionReject {
		forwardType = "REJECT"
	}

	err := NetworkPrepend(fmt.Sprintf("%s", family), name, "", "FORWARD", "-i", name, "-j", forwardType)
	if err != nil {
		return err
	}

	err = NetworkPrepend(fmt.Sprintf("%s", family), name, "", "FORWARD", "-o", name, "-j", forwardType)

	if err != nil {
		return err
	}

	return nil
}

// NetworkSetupNAT configures NAT
func (xt XTables) NetworkSetupNAT(family firewallConsts.Family, name string, location firewallConsts.Location, args ...string) error {
	if location == firewallConsts.LocationPrepend {
		err := NetworkPrepend(fmt.Sprintf("%s", family), name, "nat", "POSTROUTING", args...)
		if err != nil {
			return err
		}
	} else if location == firewallConsts.LocationAppend {
		err := NetworkAppend(fmt.Sprintf("%s", family), name, "nat", "POSTROUTING", args...)
		if err != nil {
			return err
		}
	}

	return nil
}

// NetworkSetupIPv4DNSOverrides sets up basic iptables overrides for DHCP/DNS
func (xt XTables) NetworkSetupIPv4DNSOverrides(name string) error {
	rules := [][]string{
		{"ipv4", name, "", "INPUT", "-i", name, "-p", "udp", "--dport", "67", "-j", "ACCEPT"},
		{"ipv4", name, "", "INPUT", "-i", name, "-p", "udp", "--dport", "53", "-j", "ACCEPT"},
		{"ipv4", name, "", "INPUT", "-i", name, "-p", "tcp", "--dport", "53", "-j", "ACCEPT"},
		{"ipv4", name, "", "OUTPUT", "-o", name, "-p", "udp", "--sport", "67", "-j", "ACCEPT"},
		{"ipv4", name, "", "OUTPUT", "-o", name, "-p", "udp", "--sport", "53", "-j", "ACCEPT"},
		{"ipv4", name, "", "OUTPUT", "-o", name, "-p", "tcp", "--sport", "53", "-j", "ACCEPT"}}

	for _, rule := range rules {
		err := NetworkPrepend(rule[0], rule[1], rule[2], rule[3], rule[4:]...)
		if err != nil {
			return err
		}
	}

	return nil
}

// NetworkSetupIPv4DHCPWorkaround attempts a workaround for broken DHCP clients
func (xt XTables) NetworkSetupIPv4DHCPWorkaround(name string) error {
	return NetworkPrepend("ipv4", name, "mangle", "POSTROUTING", "-o", name, "-p", "udp", "--dport", "68", "-j", "CHECKSUM", "--checksum-fill")
}

// NetworkSetupIPv6DNSOverrides sets up basic iptables overrides for DHCP/DNS
func (xt XTables) NetworkSetupIPv6DNSOverrides(name string) error {
	rules := [][]string{
		{"ipv6", name, "", "INPUT", "-i", name, "-p", "udp", "--dport", "547", "-j", "ACCEPT"},
		{"ipv6", name, "", "INPUT", "-i", name, "-p", "udp", "--dport", "53", "-j", "ACCEPT"},
		{"ipv6", name, "", "INPUT", "-i", name, "-p", "tcp", "--dport", "53", "-j", "ACCEPT"},
		{"ipv6", name, "", "OUTPUT", "-o", name, "-p", "udp", "--sport", "547", "-j", "ACCEPT"},
		{"ipv6", name, "", "OUTPUT", "-o", name, "-p", "udp", "--sport", "53", "-j", "ACCEPT"},
		{"ipv6", name, "", "OUTPUT", "-o", name, "-p", "tcp", "--sport", "53", "-j", "ACCEPT"}}

	for _, rule := range rules {
		err := NetworkPrepend(rule[0], rule[1], rule[2], rule[3], rule[4:]...)
		if err != nil {
			return err
		}
	}

	return nil
}

// NetworkSetupTunnelNAT configures tunnel NAT
func (xt XTables) NetworkSetupTunnelNAT(name string, location firewallConsts.Location, overlaySubnet net.IPNet) error {
	if location == firewallConsts.LocationPrepend {
		err := NetworkPrepend("ipv4", name, "nat", "POSTROUTING", "-s", overlaySubnet.String(), "!", "-d", overlaySubnet.String(), "-j", "MASQUERADE")
		if err != nil {
			return err
		}
	} else if location == firewallConsts.LocationAppend {
		err := NetworkAppend("ipv4", name, "nat", "POSTROUTING", "-s", overlaySubnet.String(), "!", "-d", overlaySubnet.String(), "-j", "MASQUERADE")
		if err != nil {
			return err
		}
	}

	return nil
}

// Helper Functions

// generateFilterEbtablesRules returns a customised set of ebtables filter rules based on the device.
func generateFilterEbtablesRules(m deviceConfig.Device, ipv4 net.IP, ipv6 net.IP) [][]string {
	// MAC source filtering rules. Blocks any packet coming from instance with an incorrect Ethernet source MAC.
	// This is required for IP filtering too.
	rules := [][]string{
		{"ebtables", "-t", "filter", "-A", "INPUT", "-s", "!", m["hwaddr"], "-i", m["host_name"], "-j", "DROP"},
		{"ebtables", "-t", "filter", "-A", "FORWARD", "-s", "!", m["hwaddr"], "-i", m["host_name"], "-j", "DROP"},
	}

	if shared.IsTrue(m["security.ipv4_filtering"]) && ipv4 != nil {
		rules = append(rules,
			// Prevent ARP MAC spoofing (prevents the instance poisoning the ARP cache of its neighbours with a MAC address that isn't its own).
			[]string{"ebtables", "-t", "filter", "-A", "INPUT", "-p", "ARP", "-i", m["host_name"], "--arp-mac-src", "!", m["hwaddr"], "-j", "DROP"},
			[]string{"ebtables", "-t", "filter", "-A", "FORWARD", "-p", "ARP", "-i", m["host_name"], "--arp-mac-src", "!", m["hwaddr"], "-j", "DROP"},
			// Prevent ARP IP spoofing (prevents the instance redirecting traffic for IPs that are not its own).
			[]string{"ebtables", "-t", "filter", "-A", "INPUT", "-p", "ARP", "-i", m["host_name"], "--arp-ip-src", "!", ipv4.String(), "-j", "DROP"},
			[]string{"ebtables", "-t", "filter", "-A", "FORWARD", "-p", "ARP", "-i", m["host_name"], "--arp-ip-src", "!", ipv4.String(), "-j", "DROP"},
			// Allow DHCPv4 to the host only. This must come before the IP source filtering rules below.
			[]string{"ebtables", "-t", "filter", "-A", "INPUT", "-p", "IPv4", "-s", m["hwaddr"], "-i", m["host_name"], "--ip-src", "0.0.0.0", "--ip-dst", "255.255.255.255", "--ip-proto", "udp", "--ip-dport", "67", "-j", "ACCEPT"},
			// IP source filtering rules. Blocks any packet coming from instance with an incorrect IP source address.
			[]string{"ebtables", "-t", "filter", "-A", "INPUT", "-p", "IPv4", "-i", m["host_name"], "--ip-src", "!", ipv4.String(), "-j", "DROP"},
			[]string{"ebtables", "-t", "filter", "-A", "FORWARD", "-p", "IPv4", "-i", m["host_name"], "--ip-src", "!", ipv4.String(), "-j", "DROP"},
		)
	}

	if shared.IsTrue(m["security.ipv6_filtering"]) && ipv6 != nil {
		rules = append(rules,
			// Allow DHCPv6 and Router Solicitation to the host only. This must come before the IP source filtering rules below.
			[]string{"ebtables", "-t", "filter", "-A", "INPUT", "-p", "IPv6", "-s", m["hwaddr"], "-i", m["host_name"], "--ip6-src", "fe80::/ffc0::", "--ip6-dst", "ff02::1:2/ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "--ip6-proto", "udp", "--ip6-dport", "547", "-j", "ACCEPT"},
			[]string{"ebtables", "-t", "filter", "-A", "INPUT", "-p", "IPv6", "-s", m["hwaddr"], "-i", m["host_name"], "--ip6-src", "fe80::/ffc0::", "--ip6-dst", "ff02::2/ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "--ip6-proto", "ipv6-icmp", "--ip6-icmp-type", "router-solicitation", "-j", "ACCEPT"},
			// IP source filtering rules. Blocks any packet coming from instance with an incorrect IP source address.
			[]string{"ebtables", "-t", "filter", "-A", "INPUT", "-p", "IPv6", "-i", m["host_name"], "--ip6-src", "!", fmt.Sprintf("%s/ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", ipv6.String()), "-j", "DROP"},
			[]string{"ebtables", "-t", "filter", "-A", "FORWARD", "-p", "IPv6", "-i", m["host_name"], "--ip6-src", "!", fmt.Sprintf("%s/ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", ipv6.String()), "-j", "DROP"},
		)
	}

	return rules
}

// generateFilterIptablesRules returns a customised set of iptables filter rules based on the device.
func generateFilterIptablesRules(m deviceConfig.Device, ipv6 net.IP) (rules [][]string, err error) {
	mac, err := net.ParseMAC(m["hwaddr"])
	if err != nil {
		return
	}

	macHex := hex.EncodeToString(mac)

	// These rules below are implemented using ip6tables because the functionality to inspect
	// the contents of an ICMPv6 packet does not exist in ebtables (unlike for IPv4 ARP).
	// Additionally, ip6tables doesn't really provide a nice way to do what we need here, so we
	// have resorted to doing a raw hex comparison of the packet contents at fixed positions.
	// If these rules are not added then it is possible to hijack traffic for another IP that is
	// not assigned to the instance by sending a specially crafted gratuitous NDP packet with
	// correct source address and MAC at the IP & ethernet layers, but a fraudulent IP or MAC
	// inside the ICMPv6 NDP packet.
	if shared.IsTrue(m["security.ipv6_filtering"]) && ipv6 != nil {
		ipv6Hex := hex.EncodeToString(ipv6)

		rules = append(rules,
			// Prevent Neighbor Advertisement IP spoofing (prevents the instance redirecting traffic for IPs that are not its own).
			[]string{"ipv6", "INPUT", "-i", m["parent"], "-p", "ipv6-icmp", "-m", "physdev", "--physdev-in", m["host_name"], "-m", "icmp6", "--icmpv6-type", "136", "-m", "string", "!", "--hex-string", fmt.Sprintf("|%s|", ipv6Hex), "--algo", "bm", "--from", "48", "--to", "64", "-j", "DROP"},
			[]string{"ipv6", "FORWARD", "-i", m["parent"], "-p", "ipv6-icmp", "-m", "physdev", "--physdev-in", m["host_name"], "-m", "icmp6", "--icmpv6-type", "136", "-m", "string", "!", "--hex-string", fmt.Sprintf("|%s|", ipv6Hex), "--algo", "bm", "--from", "48", "--to", "64", "-j", "DROP"},
			// Prevent Neighbor Advertisement MAC spoofing (prevents the instance poisoning the NDP cache of its neighbours with a MAC address that isn't its own).
			[]string{"ipv6", "INPUT", "-i", m["parent"], "-p", "ipv6-icmp", "-m", "physdev", "--physdev-in", m["host_name"], "-m", "icmp6", "--icmpv6-type", "136", "-m", "string", "!", "--hex-string", fmt.Sprintf("|%s|", macHex), "--algo", "bm", "--from", "66", "--to", "72", "-j", "DROP"},
			[]string{"ipv6", "FORWARD", "-i", m["parent"], "-p", "ipv6-icmp", "-m", "physdev", "--physdev-in", m["host_name"], "-m", "icmp6", "--icmpv6-type", "136", "-m", "string", "!", "--hex-string", fmt.Sprintf("|%s|", macHex), "--algo", "bm", "--from", "66", "--to", "72", "-j", "DROP"},
		)
	}

	return
}

// matchEbtablesRule compares an active rule to a supplied match rule to see if they match.
// If deleteMode is true then the "-A" flag in the active rule will be modified to "-D" and will
// not be part of the equality match. This allows delete commands to be generated from dumped add commands.
func matchEbtablesRule(activeRule []string, matchRule []string, deleteMode bool) bool {
	for i := range matchRule {
		// Active rules will be dumped in "add" format, we need to detect
		// this and switch it to "delete" mode if requested. If this has already been
		// done then move on, as we don't want to break the comparison below.
		if deleteMode && (activeRule[i] == "-A" || activeRule[i] == "-D") {
			activeRule[i] = "-D"
			continue
		}

		// Check the match rule field matches the active rule field.
		// If they don't match, then this isn't one of our rules.
		if activeRule[i] != matchRule[i] {
			return false
		}
	}

	return true
}
