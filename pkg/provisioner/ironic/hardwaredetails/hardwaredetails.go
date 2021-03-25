package hardwaredetails

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/openstack/baremetalintrospection/v1/introspection"

	metal3v1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
)

// GetHardwareDetails converts node properties into BareMetalHost HardwareDetails.
func GetHardwareDetails(node *nodes.Node) *metal3v1alpha1.HardwareDetails {
	details := new(metal3v1alpha1.HardwareDetails)
	// FIXME(levsha): Fetch the data from the conductor
	//details.Firmware = getFirmwareDetails(data.Extra.Firmware)
	//details.SystemVendor = getSystemVendorDetails(data.Inventory.SystemVendor)
	if field, ok := node.Properties["memory_mb"]; ok {
		if mem, err := strconv.Atoi(field.(string)); err == nil {
			details.RAMMebibytes = mem
		}
	}
	//details.NIC = getNICDetails(data.Inventory.Interfaces, data.AllInterfaces, data.Extra.Network)
	//details.Storage = getStorageDetails(data.Inventory.Disks)
	details.CPU = getCPUDetails(node)
	//details.Hostname = data.Inventory.Hostname
	details.Hostname = "fixme.levsha.does.not.exist" // FIXME(levsha): fill it properly or change the test.
	return details
}

func getVLANs(intf introspection.BaseInterfaceType) (vlans []metal3v1alpha1.VLAN, vlanid metal3v1alpha1.VLANID) {
	if intf.LLDPProcessed == nil {
		return
	}
	if spvs, ok := intf.LLDPProcessed["switch_port_vlans"]; ok {
		if data, ok := spvs.([]map[string]interface{}); ok {
			vlans = make([]metal3v1alpha1.VLAN, len(data))
			for i, vlan := range data {
				vid, _ := vlan["id"].(int)
				name, _ := vlan["name"].(string)
				vlans[i] = metal3v1alpha1.VLAN{
					ID:   metal3v1alpha1.VLANID(vid),
					Name: name,
				}
			}
		}
	}
	if vid, ok := intf.LLDPProcessed["switch_port_untagged_vlan_id"].(int); ok {
		vlanid = metal3v1alpha1.VLANID(vid)
	}
	return
}

func getNICSpeedGbps(intfExtradata introspection.ExtraHardwareData) (speedGbps int) {
	if speed, ok := intfExtradata["speed"].(string); ok {
		if strings.HasSuffix(speed, "Gbps") {
			fmt.Sscanf(speed, "%d", &speedGbps)
		}
	}
	return
}

func getNICDetails(ifdata []introspection.InterfaceType,
	basedata map[string]introspection.BaseInterfaceType,
	extradata introspection.ExtraHardwareDataSection) []metal3v1alpha1.NIC {
	var nics []metal3v1alpha1.NIC
	for _, intf := range ifdata {
		baseIntf := basedata[intf.Name]
		vlans, vlanid := getVLANs(baseIntf)
		// We still store one nic even if both ips are unset
		// if both are set, we store two nics with each ip
		if intf.IPV4Address != "" || intf.IPV6Address == "" {
			nics = append(nics, metal3v1alpha1.NIC{
				Name: intf.Name,
				Model: strings.TrimLeft(fmt.Sprintf("%s %s",
					intf.Vendor, intf.Product), " "),
				MAC:       intf.MACAddress,
				IP:        intf.IPV4Address,
				VLANs:     vlans,
				VLANID:    vlanid,
				SpeedGbps: getNICSpeedGbps(extradata[intf.Name]),
				PXE:       baseIntf.PXE,
			})
		}
		if intf.IPV6Address != "" {
			nics = append(nics, metal3v1alpha1.NIC{
				Name: intf.Name,
				Model: strings.TrimLeft(fmt.Sprintf("%s %s",
					intf.Vendor, intf.Product), " "),
				MAC:       intf.MACAddress,
				IP:        intf.IPV6Address,
				VLANs:     vlans,
				VLANID:    vlanid,
				SpeedGbps: getNICSpeedGbps(extradata[intf.Name]),
				PXE:       baseIntf.PXE,
			})
		}
	}
	return nics
}

func getStorageDetails(diskdata []introspection.RootDiskType) []metal3v1alpha1.Storage {
	storage := make([]metal3v1alpha1.Storage, len(diskdata))
	for i, disk := range diskdata {
		storage[i] = metal3v1alpha1.Storage{
			Name:               disk.Name,
			Rotational:         disk.Rotational,
			SizeBytes:          metal3v1alpha1.Capacity(disk.Size),
			Vendor:             disk.Vendor,
			Model:              disk.Model,
			SerialNumber:       disk.Serial,
			WWN:                disk.Wwn,
			WWNVendorExtension: disk.WwnVendorExtension,
			WWNWithExtension:   disk.WwnWithExtension,
			HCTL:               disk.Hctl,
		}
	}
	return storage
}

func getSystemVendorDetails(vendor introspection.SystemVendorType) metal3v1alpha1.HardwareSystemVendor {
	return metal3v1alpha1.HardwareSystemVendor{
		Manufacturer: vendor.Manufacturer,
		ProductName:  vendor.ProductName,
		SerialNumber: vendor.SerialNumber,
	}
}

func getCPUDetails(node *nodes.Node) (cpu metal3v1alpha1.CPU) {
	if arch, ok := node.Properties["cpu_arch"]; ok {
		cpu.Arch = arch.(string)
	}
	if field, ok := node.Properties["cpus"]; ok {
		switch v := field.(type) {
		case int:
			cpu.Count = v
		case float64:
			cpu.Count = int(v)
		case string:
			if cpus, err := strconv.Atoi(v); err == nil {
				cpu.Count = cpus
			}
		default:
			panic(fmt.Sprintf("Can't convert value type %T to cpu.Count!\n", v))
		}
	}
	return
}

func getFirmwareDetails(firmwaredata introspection.ExtraHardwareDataSection) metal3v1alpha1.Firmware {

	// handle bios optionally
	var bios metal3v1alpha1.BIOS

	if biosdata, ok := firmwaredata["bios"]; ok {
		// we do not know if all fields will be supplied
		// as this is not a structured response
		// so we must handle each field conditionally
		bios.Vendor, _ = biosdata["vendor"].(string)
		bios.Version, _ = biosdata["version"].(string)
		bios.Date, _ = biosdata["date"].(string)
	}

	return metal3v1alpha1.Firmware{
		BIOS: bios,
	}

}
