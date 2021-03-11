package provider

// import (
// 	"fmt"

// 	"gitlab.dds-sysu.tech/Wuny/kmon/src/model"
// )

// type Direct struct {
// 	src    *model.ProviderConfig
// 	update bool
// }

// func init() {
// 	providers["direct"] = NewDirect
// }

// func NewDirect(src *model.ProviderConfig) (model.Provider, error) {
// 	return &Direct{src: src, update: true}, nil
// }

// func (pipe *Direct) GetInt() (int, error) {
// 	integer, ok := pipe.src.Param.(int)
// 	if !ok {
// 		return 0, fmt.Errorf("Cannot parse int param")
// 	}
// 	return integer, nil
// }

// func (pipe *Direct) GetBool() (bool, error) {
// 	boolean, ok := pipe.src.Param.(bool)
// 	if !ok {
// 		return false, fmt.Errorf("Cannot parse int param")
// 	}
// 	return boolean, nil
// }

// func (pipe *Direct) GetString() (string, error) {
// 	str, ok := pipe.src.Param.(string)
// 	if !ok {
// 		return "", fmt.Errorf("Cannot parse string param")
// 	}
// 	return str, nil
// }

// func (pipe *Direct) GetArrayUint32() ([]uint32, error) {
// 	arr, ok := pipe.src.Param.([]interface{})
// 	if !ok {
// 		return nil, fmt.Errorf("Cannot parse uint array param")
// 	}

// 	uintArr := make([]uint32, len(arr))
// 	for idx, data := range arr {
// 		if intData, ok := data.(int); ok {
// 			uintArr[idx] = uint32(intData)
// 		} else {
// 			return nil, fmt.Errorf("Cannot parse uint array element")
// 		}
// 	}

// 	return uintArr, nil
// }

// func (pipe *Direct) IsUpdated() bool {
// 	if pipe.update {
// 		pipe.update = false
// 		return true
// 	}
// 	return false
// }

// func (pipe *Direct) AppendAdditionalData(data *model.ExportData) {
// 	return
// }
