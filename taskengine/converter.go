package taskengine

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// ConvertToType converts a value to the specified DataType
func ConvertToType(value interface{}, dataType DataType) (interface{}, error) {
	switch dataType {
	case DataTypeChatHistory:
		return convertToChatHistory(value)
	case DataTypeOpenAIChat:
		return convertToOpenAIChatRequest(value)
	case DataTypeOpenAIChatResponse:
		return convertToOpenAIChatResponse(value)
	case DataTypeSearchResults:
		return convertToSearchResults(value)
	case DataTypeString:
		return convertToString(value)
	case DataTypeBool:
		return convertToBool(value)
	case DataTypeInt:
		return convertToInt(value)
	case DataTypeFloat:
		return convertToFloat(value)
	case DataTypeVector:
		return convertToFloatSlice(value)
	case DataTypeJSON:
		return value, nil // Already in generic JSON form
	default:
		return value, nil // For DataTypeAny, return as-is
	}
}

func convertToChatHistory(value interface{}) (ChatHistory, error) {
	switch v := value.(type) {
	case ChatHistory:
		return v, nil
	case map[string]interface{}:
		data, err := json.Marshal(v)
		if err != nil {
			return ChatHistory{}, err
		}
		var hist ChatHistory
		err = json.Unmarshal(data, &hist)
		return hist, err
	default:
		return ChatHistory{}, fmt.Errorf("cannot convert %T to ChatHistory", value)
	}
}

func convertToOpenAIChatRequest(value interface{}) (OpenAIChatRequest, error) {
	switch v := value.(type) {
	case OpenAIChatRequest:
		return v, nil
	case map[string]interface{}:
		data, err := json.Marshal(v)
		if err != nil {
			return OpenAIChatRequest{}, err
		}
		var req OpenAIChatRequest
		err = json.Unmarshal(data, &req)
		return req, err
	default:
		return OpenAIChatRequest{}, fmt.Errorf("cannot convert %T to OpenAIChatRequest", value)
	}
}

func convertToOpenAIChatResponse(value interface{}) (OpenAIChatResponse, error) {
	switch v := value.(type) {
	case OpenAIChatResponse:
		return v, nil
	case map[string]interface{}:
		data, err := json.Marshal(v)
		if err != nil {
			return OpenAIChatResponse{}, err
		}
		var resp OpenAIChatResponse
		err = json.Unmarshal(data, &resp)
		return resp, err
	default:
		return OpenAIChatResponse{}, fmt.Errorf("cannot convert %T to OpenAIChatResponse", value)
	}
}

func convertToSearchResults(value interface{}) ([]SearchResult, error) {
	switch v := value.(type) {
	case []SearchResult:
		return v, nil
	case []interface{}:
		results := make([]SearchResult, len(v))
		for i, item := range v {
			if sr, ok := item.(SearchResult); ok {
				results[i] = sr
			} else if m, ok := item.(map[string]interface{}); ok {
				data, err := json.Marshal(m)
				if err != nil {
					return nil, err
				}
				var sr SearchResult
				if err := json.Unmarshal(data, &sr); err != nil {
					return nil, err
				}
				results[i] = sr
			} else {
				return nil, fmt.Errorf("invalid search result type: %T", item)
			}
		}
		return results, nil
	default:
		return nil, fmt.Errorf("cannot convert %T to []SearchResult", value)
	}
}

// Basic type conversions
func convertToString(value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

func convertToBool(value interface{}) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		return strconv.ParseBool(v)
	case int, float64:
		return v != 0, nil
	default:
		return false, fmt.Errorf("cannot convert %T to bool", value)
	}
}

func convertToInt(value interface{}) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case float64:
		return int(v), nil
	case string:
		return strconv.Atoi(v)
	default:
		return 0, fmt.Errorf("cannot convert %T to int", value)
	}
}

func convertToFloat(value interface{}) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float", value)
	}
}

func convertToFloatSlice(value interface{}) ([]float64, error) {
	switch v := value.(type) {
	case []float64:
		return v, nil
	case []interface{}:
		floats := make([]float64, len(v))
		for i, item := range v {
			if f, ok := item.(float64); ok {
				floats[i] = f
			} else {
				return nil, fmt.Errorf("element %d is %T, not float64", i, item)
			}
		}
		return floats, nil
	default:
		return nil, fmt.Errorf("cannot convert %T to []float64", value)
	}
}
