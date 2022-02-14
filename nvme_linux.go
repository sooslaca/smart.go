package smart

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// https:// nvmexpress.org/wp-content/uploads/NVM-Express-Base-Specification-2.0b-2021.12.18-Ratified.pdf
// https:// nvmexpress.org/wp-content/uploads/NVM-Express-NVM-Command-Set-Specification-1.0b-2021.12.18-Ratified.pdf

// include/uapi/linux/nvme_ioctl.h

var nvmeIoctlAdmin64Cmd = iowr('N', 0x47, unsafe.Sizeof(nvmePassthruCmd64{}))

type nvmePassthruCmd64 struct {
	opcode      uint8
	flags       uint8
	_           uint16
	nsid        uint32
	cdw2        uint32
	cdw3        uint32
	metadata    uint64
	addr        uint64
	metadataLen uint32
	dataLen     uint32
	cdw10       uint32
	cdw11       uint32
	cdw12       uint32
	cdw13       uint32
	cdw14       uint32
	cdw15       uint32
	timeoutMs   uint32
	_           uint32
	result      uint64
}

func OpenNVMe(name string) (*NVMeDevice, error) {
	fd, err := unix.Open(name, unix.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	dev := NVMeDevice{
		fd: fd,
	}
	return &dev, nil
}

func (d *NVMeDevice) Close() error {
	return unix.Close(d.fd)
}

func (d *NVMeDevice) Identify() (*NvmeIdentController, []NvmeIdentNamespace, error) {
	buf := make([]byte, 4096)
	if err := nvmeReadIdentify(d.fd, 0, 1, buf); err != nil {
		return nil, nil, err
	}
	var controller NvmeIdentController
	if err := binary.Read(bytes.NewBuffer(buf), binary.LittleEndian, &controller); err != nil {
		return nil, nil, err
	}

	var ns []NvmeIdentNamespace
	// QEMU has 256 namespaces for some reason, TODO: clarify
	for i := 0; i < int(controller.Nn); i++ {
		buf2 := make([]byte, 4096)
		var n NvmeIdentNamespace
		if err := nvmeReadIdentify(d.fd, uint32(i+1), 0, buf2); err != nil {
			return nil, nil, err
		}
		if err := binary.Read(bytes.NewBuffer(buf2), binary.LittleEndian, &n); err != nil {
			return nil, nil, err
		}
		if n.Nsze == 0 {
			continue
		}

		ns = append(ns, n)
	}

	return &controller, ns, nil
}

func (d *NVMeDevice) ReadSMART() (*NvmeSMARTLog, error) {
	buf3 := make([]byte, 512)
	if err := nvmeReadLogPage(d.fd, nvmeLogSmartInformation, buf3); err != nil {
		return nil, err
	}
	var sl NvmeSMARTLog
	if err := binary.Read(bytes.NewBuffer(buf3), binary.LittleEndian, &sl); err != nil {
		return nil, err
	}

	return &sl, nil
}

func nvmeReadLogPage(fd int, logID uint8, buf []byte) error {
	bufLen := len(buf)

	if (bufLen < 4) || (bufLen > 0x4000) || (bufLen%4 != 0) {
		return fmt.Errorf("invalid buffer size")
	}

	cmd := nvmePassthruCmd64{
		opcode:  nvmeAdminGetLogPage,
		nsid:    0xffffffff, // controller-level SMART info
		addr:    uint64(uintptr(unsafe.Pointer(&buf[0]))),
		dataLen: uint32(bufLen),
		cdw10:   uint32(logID) | (((uint32(bufLen) / 4) - 1) << 16),
	}

	return ioctl(uintptr(fd), nvmeIoctlAdmin64Cmd, uintptr(unsafe.Pointer(&cmd)))
}

func nvmeReadIdentify(fd int, nsid, cns uint32, data []byte) error {
	cmd := nvmePassthruCmd64{
		opcode:  nvmeAdminIdentify,
		nsid:    nsid,
		addr:    uint64(uintptr(unsafe.Pointer(&data[0]))),
		dataLen: uint32(len(data)),
		cdw10:   cns,
	}

	return ioctl(uintptr(fd), nvmeIoctlAdmin64Cmd, uintptr(unsafe.Pointer(&cmd)))
}