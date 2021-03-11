// package model

// import (
// 	"bytes"
// 	"encoding/binary"
// 	"fmt"
// 	"regexp"
// 	"strconv"
// 	"unsafe"

// 	"gitlab.dds-sysu.tech/Wuny/kmon/src/utils"
// )

// const (
// 	Unknown = 0
// 	Int     = 1
// 	Ptr     = 2
// 	Array   = 3
// 	Struct  = 4
// )

// const (
// 	Signed = 1 << 0
// 	Char   = 1 << 1
// 	Bool   = 1 << 2
// )

// const ByteSize = 8

// type MapType struct {
// 	BtfType   int
// 	IntType   int
// 	IntBits   int
// 	IntOffset int
// 	Offset    int
// 	ByteSize  int64
// 	Member    []*MapType
// 	Name      string
// }

// func ParseDataStruct(data interface{}) (*MapType, error) {
// 	mapType := &MapType{}
// 	if structData, ok := data.(map[string]interface{}); ok {
// 		mapType.BtfType = Struct

// 		offset := 0
// 		for subName, subData := range structData {
// 			if subMapData, err := ParseDataStruct(subData); err == nil {
// 				subMapData.Offset = offset
// 				subMapData.Name = subName
// 				offset += int(subMapData.ByteSize)

// 				mapType.Member = append(mapType.Member, subMapData)
// 			} else {
// 				return nil, fmt.Errorf("parse data struct error: %w", err)
// 			}
// 		}
// 		mapType.ByteSize = int64(offset)
// 	} else if typeData, ok := data.(string); ok {
// 		exp := regexp.MustCompile("(.+)\\((\\d+)\\)")
// 		param := exp.FindStringSubmatch(typeData)
// 		if len(param) != 2 {
// 			return nil, fmt.Errorf("Incorrrect config string struct")
// 		}

// 		name := param[0]
// 		size, err := strconv.Atoi(param[1])
// 		if err != nil {
// 			return nil, fmt.Errorf("convert string to int error: %w", err)
// 		}

// 		if name == "string" {
// 			subType := &MapType{
// 				BtfType:   Int,
// 				IntBits:   8,
// 				IntType:   Signed,
// 				IntOffset: 0,
// 				ByteSize:  8,
// 			}
// 			mapType.BtfType = Array
// 			mapType.ByteSize = int64(size)
// 			mapType.Member = []*MapType{subType}
// 		} else {
// 			intBits := 0
// 			intType := 0

// 			switch name {
// 			case "int":
// 			case "uint":
// 			case "bool":
// 				intBits = 32
// 			case "long":
// 			case "ulong":
// 				intBits = 64
// 			case "short":
// 			case "ushort":
// 				intBits = 16
// 			case "byte":
// 			case "char":
// 				intBits = 8
// 			}

// 			switch name {
// 			case "int":
// 			case "long":
// 			case "short":
// 			case "char":
// 				intType &= Signed
// 			}

// 			mapType.BtfType = Array
// 			mapType.ByteSize = int64(size)
// 			mapType.IntBits = intBits
// 			mapType.IntType = intType
// 		}
// 	}

// 	return mapType, nil
// }

// func ParseData(data []byte, mapType *MapType) interface{} {
// 	switch mapType.BtfType {
// 	case Int:
// 		begin := mapType.Offset / ByteSize
// 		end := (mapType.Offset + mapType.IntOffset) / ByteSize
// 		subData := data[begin:end]
// 		if mapType.IntType&Bool > 0 { // 布尔值
// 			return *(*bool)(unsafe.Pointer(&subData[0]))
// 		}
// 		if mapType.IntType&Char > 0 { // 字符
// 			return *(*byte)(unsafe.Pointer(&subData[0]))
// 		}
// 		if mapType.IntType&Signed > 0 { //有符号数
// 			switch mapType.IntBits {
// 			case 8:
// 				return *(*int8)(unsafe.Pointer(&subData[0]))
// 			case 16:
// 				return *(*int16)(unsafe.Pointer(&subData[0]))
// 			case 32:
// 				return *(*int32)(unsafe.Pointer(&subData[0]))
// 			case 64:
// 				return *(*int64)(unsafe.Pointer(&subData[0]))
// 			}
// 		} else { //无符号数
// 			switch mapType.IntBits {
// 			case 8:
// 				return *(*uint8)(unsafe.Pointer(&subData[0]))
// 			case 16:
// 				return *(*uint16)(unsafe.Pointer(&subData[0]))
// 			case 32:
// 				return *(*uint32)(unsafe.Pointer(&subData[0]))
// 			case 64:
// 				return *(*uint64)(unsafe.Pointer(&subData[0]))
// 			}
// 		}
// 	case Array:
// 		member := mapType.Member[0]
// 		begin := mapType.Offset / ByteSize
// 		end := (int64(mapType.Offset) + mapType.ByteSize*ByteSize) / ByteSize
// 		subData := data[begin:end]

// 		if member.BtfType == Int {
// 			buffer := bytes.NewBuffer(subData)
// 			arrLen := mapType.ByteSize * ByteSize / int64(member.IntOffset)

// 			if member.IntType&Bool > 0 { // 布尔值
// 				arr := make([]bool, arrLen)
// 				binary.Read(buffer, utils.GetHostEndian(), &arr)
// 				return arr
// 			}
// 			if member.IntType&Char > 0 { // 字符
// 				arr := make([]byte, arrLen)
// 				copy(arr, subData)
// 				return arr
// 			}
// 			if member.IntType&Signed > 0 { //有符号数
// 				switch member.IntBits {
// 				case 8:
// 					arr := make([]byte, arrLen)
// 					copy(arr, subData)
// 					return arr
// 				case 16:
// 					arr := make([]int16, arrLen)
// 					binary.Read(buffer, utils.GetHostEndian(), &arr)
// 					return arr
// 				case 32:
// 					arr := make([]int32, arrLen)
// 					binary.Read(buffer, utils.GetHostEndian(), &arr)
// 					return arr
// 				case 64:
// 					arr := make([]int64, arrLen)
// 					binary.Read(buffer, utils.GetHostEndian(), &arr)
// 					return arr
// 				}
// 			} else { //无符号数
// 				switch member.IntBits {
// 				case 8:
// 					arr := make([]uint8, arrLen)
// 					binary.Read(buffer, binary.LittleEndian, &arr)
// 					return arr
// 				case 16:
// 					arr := make([]uint16, arrLen)
// 					binary.Read(buffer, binary.LittleEndian, &arr)
// 					return arr
// 				case 32:
// 					arr := make([]uint32, arrLen)
// 					binary.Read(buffer, binary.LittleEndian, &arr)
// 					return arr
// 				case 64:
// 					arr := make([]uint64, arrLen)
// 					binary.Read(buffer, utils.GetHostEndian(), &arr)
// 					return arr
// 				}
// 			}
// 		} else {
// 			arrLen := mapType.ByteSize * ByteSize / int64(member.ByteSize)
// 			arr := make([]interface{}, 0, arrLen)
// 			var i int64
// 			for i = 0; i < arrLen; i++ {
// 				arr[i] = ParseData(data[:member.ByteSize*i], member)
// 			}
// 			return arr
// 		}
// 	case Struct:
// 		mapData := make(map[string]interface{})
// 		begin := mapType.Offset / ByteSize
// 		end := (int64(mapType.Offset) + mapType.ByteSize*ByteSize) / ByteSize
// 		for _, member := range mapType.Member {
// 			mapData[member.Name] = ParseData(data[begin:end], member)
// 		}
// 		return mapData
// 	}

// 	return nil
// }
