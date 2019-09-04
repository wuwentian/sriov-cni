package sriovnet

import (
	"fmt"
	"github.com/satori/go.uuid"
	"github.com/vishvananda/netlink"
	"net"
	"path/filepath"
	"strconv"
	"strings"
)

type VfObj struct {
	Index      int
	PcidevName string
	NetdevName string
	Bound      bool
	Allocated  bool
}

type PfNetdevHandle struct {
	PfNetdevName string
	pfLinkHandle netlink.Link

	List []*VfObj
}

func SetPFLinkUp(pfNetdevName string) error {
	handle, err := netlink.LinkByName(pfNetdevName)
	if err != nil {
		return err
	}

	return netlink.LinkSetUp(handle)
}

func IsSriovSupported(netdevName string) bool {

	maxvfs, err := getMaxVfCount(netdevName)
	if maxvfs == 0 || err != nil {
		return false
	} else {
		return true
	}
}

func EnableSriov(pfNetdevName string) error {
	var maxVfCount int
	var err error

	devDirName := netDevDeviceDir(pfNetdevName)

	devExist := dirExists(devDirName)
	if !devExist {
		return fmt.Errorf("device %s not found", pfNetdevName)
	}

	maxVfCount, err = getMaxVfCount(pfNetdevName)
	if err != nil {
		fmt.Println("Fail to read max vf count of PF %v", pfNetdevName)
		return err
	}

	if maxVfCount == 0 {
		return fmt.Errorf("sriov unsupported for device: ", pfNetdevName)
	}

	curVfCount, err2 := netdevGetEnabledVfCount(pfNetdevName)
	if err2 != nil {
		fmt.Println("Fail to read current vf count of PF %v", pfNetdevName)
		return err
	}
	if curVfCount == 0 {
		return setMaxVfCount(pfNetdevName, maxVfCount)
	}
	return nil
}

func DisableSriov(pfNetdevName string) error {
	devDirName := netDevDeviceDir(pfNetdevName)

	devExist := dirExists(devDirName)
	if !devExist {
		return fmt.Errorf("device %s not found", pfNetdevName)
	}

	return setMaxVfCount(pfNetdevName, 0)
}

func GetPfNetdevHandle(pfNetdevName string) (*PfNetdevHandle, error) {

	pfLinkHandle, err := netlink.LinkByName(pfNetdevName)
	if err != nil {
		return nil, err
	}

	handle := PfNetdevHandle{
		PfNetdevName: pfNetdevName,
		pfLinkHandle: pfLinkHandle,
	}

	list, err := getVfPciDevList(pfNetdevName)
	if err != nil {
		return nil, err
	}

	for _, vfDir := range list {
		vfIndexStr := strings.TrimPrefix(vfDir, "virtfn")
		vfIndex, _ := strconv.Atoi(vfIndexStr)
		vfNetdevName := vfNetdevNameFromParent(pfNetdevName, vfDir)
		vfObj := VfObj{
			Index:      vfIndex,
			PcidevName: vfDir,
		}
		if vfNetdevName != "" {
			vfObj.NetdevName = vfNetdevName
			vfObj.Bound = true
		} else {
			vfObj.Bound = false
		}
		vfObj.Allocated = false
		handle.List = append(handle.List, &vfObj)
	}
	return &handle, nil
}

func UnbindVf(handle *PfNetdevHandle, vf *VfObj) error {
	cmdFile := filepath.Join(netSysDir, handle.PfNetdevName, netdevDriverDir, netdevUnbindFile)
	cmdFileObj := fileObject{
		Path: cmdFile,
	}

	pciDevName := vfPCIDevNameFromVfDir(handle.PfNetdevName, vf.PcidevName)
	err := cmdFileObj.Write(pciDevName)
	if err != nil {
		vf.Bound = false
		vf.NetdevName = ""
	}
	return err
}

func BindVf(handle *PfNetdevHandle, vf *VfObj) error {
	cmdFile := filepath.Join(netSysDir, handle.PfNetdevName, netdevDriverDir, netdevBindFile)
	cmdFileObj := fileObject{
		Path: cmdFile,
	}

	pciDevName := vfPCIDevNameFromVfDir(handle.PfNetdevName, vf.PcidevName)
	err := cmdFileObj.Write(pciDevName)
	if err != nil {
		vf.Bound = true
		vf.NetdevName = vfNetdevNameFromParent(handle.PfNetdevName, vf.PcidevName)
	}
	return err
}

func GetVfDefaultMacAddr(vfNetdevName string) (string, error) {

	ethHandle, err1 := netlink.LinkByName(vfNetdevName)
	if err1 != nil {
		return "", err1
	}

	ethAttr := ethHandle.Attrs()
	return ethAttr.HardwareAddr.String(), nil
}

func SetVfDefaultMacAddress(handle *PfNetdevHandle, vf *VfObj) error {
	ethHandle, err1 := netlink.LinkByName(vf.NetdevName)
	if err1 != nil {
		return err1
	}
	ethAttr := ethHandle.Attrs()
	return netlink.LinkSetVfHardwareAddr(handle.pfLinkHandle, vf.Index, ethAttr.HardwareAddr)
}

func SetVfVlan(handle *PfNetdevHandle, vf *VfObj, vlan int) error {
	return netlink.LinkSetVfVlan(handle.pfLinkHandle, vf.Index, vlan)
}

func setVfNodeGuid(handle *PfNetdevHandle, vf *VfObj, guid []byte) error {
	var err error

	nodeGuidHwAddr := net.HardwareAddr(guid)

	err = ibSetNodeGuid(handle.PfNetdevName, vf.Index, nodeGuidHwAddr)
	if err == nil {
		return nil
	}
	err = netlink.LinkSetVfNodeGUID(handle.pfLinkHandle, vf.Index, guid)
	return err
}

func setVfPortGuid(handle *PfNetdevHandle, vf *VfObj, guid []byte) error {
	var err error

	portGuidHwAddr := net.HardwareAddr(guid)

	err = ibSetPortGuid(handle.PfNetdevName, vf.Index, portGuidHwAddr)
	if err == nil {
		return nil
	}
	err = netlink.LinkSetVfPortGUID(handle.pfLinkHandle, vf.Index, guid)
	return err
}

func SetVfDefaultGUID(handle *PfNetdevHandle, vf *VfObj) error {

	uuid, err := uuid.NewV4()
	if err != nil {
		return err
	}
	guid := uuid[0:8]
	guid[7] = byte(vf.Index)

	err = setVfNodeGuid(handle, vf, guid)
	if err != nil {
		return err
	}

	err = setVfPortGuid(handle, vf, guid)
	return err
}

func SetVfPrivileged(handle *PfNetdevHandle, vf *VfObj, privileged bool) error {

	var spoofChk bool
	var trusted bool

	ethAttr := handle.pfLinkHandle.Attrs()
	if ethAttr.EncapType != "ether" {
		return nil
	}
	//Only ether type is supported
	if privileged {
		spoofChk = false
		trusted = true
	} else {
		spoofChk = true
		trusted = false
	}

	/* do not check for error status as older kernels doesn't
	 * have support for it.
	 */
	netlink.LinkSetVfTrust(handle.pfLinkHandle, vf.Index, trusted)
	netlink.LinkSetVfSpoofchk(handle.pfLinkHandle, vf.Index, spoofChk)
	return nil
}

func setDefaultHwAddr(handle *PfNetdevHandle, vf *VfObj) error {
	var err error

	ethAttr := handle.pfLinkHandle.Attrs()
	if ethAttr.EncapType == "ether" {
		err = SetVfDefaultMacAddress(handle, vf)
	} else if ethAttr.EncapType == "infiniband" {
		err = SetVfDefaultGUID(handle, vf)
	}
	return err
}

func setPortAdminState(handle *PfNetdevHandle, vf *VfObj) error {
	ethAttr := handle.pfLinkHandle.Attrs()
	if ethAttr.EncapType == "infiniband" {
		state, err2 := ibGetPortAdminState(handle.PfNetdevName, vf.Index)
		// Ignore the error where this file is not available
		if err2 != nil {
			return nil
		}
		fmt.Printf("Admin state = %v", state)
		err2 = ibSetPortAdminState(handle.PfNetdevName, vf.Index, ibSriovPortAdminStateFollow)
		if err2 != nil {
			//If file exist, we must be able to write
			fmt.Printf("Admin state setting error = %v", err2)
			return err2
		}
	}
	return nil
}

func ConfigVfs(handle *PfNetdevHandle, privileged bool) error {
	var err error

	for _, vf := range handle.List {
		fmt.Printf("vf = %v\n", vf)
		err = setPortAdminState(handle, vf)
		if err != nil {
			break
		}

		err = setDefaultHwAddr(handle, vf)
		if err != nil {
			break
		}
		_ = SetVfPrivileged(handle, vf, privileged)
		if vf.Bound {
			err = UnbindVf(handle, vf)
			if err != nil {
				fmt.Printf("Fail to unbind err=%v\n", err)
				break
			}
			err = BindVf(handle, vf)
			if err != nil {
				fmt.Printf("Fail to bind err=%v\n", err)
				break
			}
		}
	}
	return nil
}

func AllocateVf(handle *PfNetdevHandle) (*VfObj, error) {
	for _, vf := range handle.List {
		if vf.Allocated == true {
			continue
		}
		vf.Allocated = true
		fmt.Printf("Allocated vf = %v\n", *vf)
		return vf, nil
	}
	return nil, fmt.Errorf("All Vfs for %v are allocated.", handle.PfNetdevName)
}

func AllocateVfByMacAddress(handle *PfNetdevHandle, vfMacAddress string) (*VfObj, error) {
	for _, vf := range handle.List {
		if vf.Allocated == true {
			continue
		}
		macAddr, _ := GetVfDefaultMacAddr(vf.NetdevName)
		if macAddr != vfMacAddress {
			continue
		}
		vf.Allocated = true
		fmt.Printf("Allocated vf by mac = %v\n", *vf)
		return vf, nil
	}
	return nil, fmt.Errorf("All Vfs for %v are allocated.", handle.PfNetdevName)
}

func FreeVf(handle *PfNetdevHandle, vf *VfObj) {
	vf.Allocated = false
	fmt.Printf("Free vf = %v\n", *vf)
}

func FreeVfByNetdevName(handle *PfNetdevHandle, vfNetdevName string) error {
	for _, vf := range handle.List {
		if vf.Allocated == true && vf.NetdevName == vfNetdevName {
			vf.Allocated = true
			return nil
		}
	}
	return fmt.Errorf("vf netdev %v not found", vfNetdevName)
}
