package utils

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"unsafe"
)

var nativeEndian binary.ByteOrder

const UnknowFunctionName = "[unknown function]"

type ksym struct {
	Addr uint64
	Name string
}

var ksymsList []*ksym = make([]*ksym, 0)
var ksymsProbe map[string]bool = make(map[string]bool)

func init() {
	buf := [2]byte{}
	*(*uint16)(unsafe.Pointer(&buf[0])) = uint16(0xABCD)

	switch buf {
	case [2]byte{0xCD, 0xAB}:
		nativeEndian = binary.LittleEndian
	case [2]byte{0xAB, 0xCD}:
		nativeEndian = binary.BigEndian
	default:
		panic("Could not determine native endianness.")
	}

	file, err := os.Open("/proc/kallsyms")
	defer file.Close()
	if err != nil {
		panic("cannot open /proc/kallsyms")
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		ar := strings.Split(scanner.Text(), " ")
		if len(ar) != 3 {
			continue
		}

		addr, err := strconv.ParseUint(ar[0], 16, 64)
		if err != nil {
			continue
		}
		ksymsList = append(ksymsList, &ksym{
			Addr: addr,
			Name: ar[2],
		})
		ksymsProbe[ar[2]] = true
	}
	sort.Slice(ksymsList, func(i, j int) bool { return ksymsList[i].Addr < ksymsList[j].Addr })
}

func GetHostEndian() binary.ByteOrder {
	return nativeEndian
}

func StringToArrayUint32(strs []string) []uint32 {
	nums := []uint32{}
	for _, i := range strs {
		j, err := strconv.Atoi(i)
		if err != nil {
			panic(err)
		}
		nums = append(nums, uint32(j))
	}
	return nums
}
func ToByte(data interface{}) ([]byte, error) {
	buffer := bytes.NewBuffer(nil)
	err := binary.Write(buffer, GetHostEndian(), data)
	if err != nil {
		return nil, fmt.Errorf("tobyte error: %w", err)
	}

	return buffer.Bytes(), nil
}
func Htons(i uint16) uint16 {
	if nativeEndian != binary.BigEndian {
		return i
	}
	return (i<<8)&0xff00 | i>>8
}

func UInt32ToIP(intIP uint32) net.IP {
	var bytes [4]byte
	if nativeEndian == binary.BigEndian {
		bytes[3] = byte(intIP & 0xFF)
		bytes[2] = byte((intIP >> 8) & 0xFF)
		bytes[1] = byte((intIP >> 16) & 0xFF)
		bytes[0] = byte((intIP >> 24) & 0xFF)
	} else {
		bytes[0] = byte(intIP & 0xFF)
		bytes[1] = byte((intIP >> 8) & 0xFF)
		bytes[2] = byte((intIP >> 16) & 0xFF)
		bytes[3] = byte((intIP >> 24) & 0xFF)
	}

	return net.IPv4(bytes[0], bytes[1], bytes[2], bytes[3])
}

func UInt128ToIP(h uint64, l uint64) net.IP {
	var bytes [16]byte
	if nativeEndian == binary.BigEndian {
		for i := 0; i < 8; i++ {
			bytes[7-i] = byte((h >> (i * 4) & 0xF))
		}
		for i := 0; i < 8; i++ {
			bytes[15-i] = byte((l >> (i * 4) & 0xF))
		}
	} else {
		for i := 0; i < 8; i++ {
			bytes[i] = byte((h >> (i * 4) & 0xF))
		}
		for i := 0; i < 8; i++ {
			bytes[i] = byte((l >> (i * 4) & 0xF))
		}
	}

	return net.IP(bytes[:])
}

// https://stackoverflow.com/questions/11376288/fast-computing-of-log2-for-64-bit-integers
var logTable64 []uint32 = []uint32{
	63, 0, 58, 1, 59, 47, 53, 2,
	60, 39, 48, 27, 54, 33, 42, 3,
	61, 51, 37, 40, 49, 18, 28, 20,
	55, 30, 34, 11, 43, 14, 22, 4,
	62, 57, 46, 52, 38, 26, 32, 41,
	50, 36, 17, 19, 29, 10, 13, 21,
	56, 45, 25, 31, 35, 16, 9, 12,
	44, 24, 15, 8, 23, 7, 6, 5,
}

func Log2Uint64(value uint64) uint32 {
	if value == 0 {
		// 原算法返回63，但按照实际需求这里返回0会更好
		return 0
	}
	value |= value >> 1
	value |= value >> 2
	value |= value >> 4
	value |= value >> 8
	value |= value >> 16
	value |= value >> 32
	index := ((value - (value >> 1)) * 0x07EDD5E59A4E28C2) >> 58
	return logTable64[index]
}

func Ksym(addr uint64) string {
	if len(ksymsList) == 0 {
		return UnknowFunctionName
	}
	// 和一般的二分法有细微的不同
	// 当地址大于等于左侧，小于右侧的时候，我们要取左侧的索引

	left, right := 0, len(ksymsList)-1
	if addr < ksymsList[left].Addr || addr > ksymsList[right].Addr {
		return UnknowFunctionName
	}

	for left < right {
		mid := right - (right-left)/2
		midAddr := ksymsList[mid].Addr
		if midAddr == addr {
			return ksymsList[mid].Name
		} else if addr < midAddr {
			right = mid - 1
		} else {
			left = mid
		}
	}

	return ksymsList[left].Name
}

func KprobeExist(funcName string) bool {
	_, ok := ksymsProbe[funcName]
	return ok
}
