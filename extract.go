// Package validator implements validation and sanitization of input.
// Written to be used in web applications, where one must make sure that the input is well behaving before
// inserting to database.
//
// Possible gotcha: encoding/json.Unmarshal uses float64 for all numbers.
//
// For now, the design is to do NOT handle the incoming data gracefully, and say no at the slightest problem.
// Eg: quit if a slice is larger than the expected instead of truncating it, etc...
// Later there will be a flag to do the opposite.
//
// I am really unhappy about the solutions in this code though (too much redundant typing). A better solution must exists.
// János Dobronszki @ Opesun
package extract

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
)

const (
	min        = "min"
	max        = "max"
	min_amt    = "min_amt"
	max_amt    = "max_amt"
	single_err = "Field \"%s\" not passed."
	slice_err  = "Slice field \"%s\" not passed."
)

type Rules struct {
	R map[string]interface{}
}

func (r *Rules) ExtractForm(dat url.Values) (map[string]interface{}, error) {
	return r.Extract(map[string][]string(dat))
}

func minMax(i int64, rules map[string]interface{}) bool {
	if min, hasmin := rules[min]; hasmin {
		switch val := min.(type) {
		case float64:
			return i >= int64(val)
		case int:
			return i >= int64(val)
		default:
			panic("Unkown min type at extract.")
		}
	}
	if max, hasmax := rules[max]; hasmax {
		switch val := max.(type) {
		case float64:
			return i <= int64(val)
		case int:
			return i <= int64(val)
		default:
			panic("Unkown max type at extract.")
		}
	}
	return true
}

func handleString(val string, rules map[string]interface{}) (string, bool) {
	len_ok := minMax(int64(len(val)), rules)
	if !len_ok {
		return val, false
	}
	return val, true
}

func handleInt(val string, rules map[string]interface{}) (int64, bool) {
	i, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, false
	}
	size_ok := minMax(i, rules) // This is so uncorrect. TODO: rethink
	if !size_ok {
		return i, false
	}
	return i, true
}

func handleFloat(val string, rules map[string]interface{}) (float64, bool) {
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0, false
	}
	size_ok := minMax(int64(math.Ceil(f)), rules) // This is so uncorrect. TODO: rethink
	if !size_ok {
		return f, false
	}
	return f, true
}

// TODO: rethink
func handleBool(val string, rules map[string]interface{}) (bool, bool) {
	b, err := strconv.ParseBool(val)
	if err != nil {
		return false, false
	}
	return b, true
}

// Slices

func minMaxS(l int, rules map[string]interface{}) bool {
	if min_amt, has_min := rules[min_amt]; has_min {
		if l < int(min_amt.(float64)) {
			return false
		}
	}
	if max_amt, has_max := rules[max_amt]; has_max {
		if l > int(max_amt.(float64)) {
			return false
		}
	}
	return true
}

func allOk(val []string, rules map[string]interface{}, f func(int) bool) bool {
	slen_ok := minMaxS(len(val), rules)
	if !slen_ok {
		return false
	}
	for i, _ := range val {
		if !f(i) {
			return false
		}
	}
	return true
}

func handleStringS(val []string, rules map[string]interface{}) ([]string, bool) {
	ret := []string{}
	return ret, allOk(val, rules,
		func(i int) bool {
			if str, ok := handleString(val[i], rules); ok {
				ret = append(ret, str)
				return true
			}
			return false
		})
}

func handleIntS(val []string, rules map[string]interface{}) ([]int64, bool) {
	ret := []int64{}
	return ret, allOk(val, rules,
		func(i int) bool {
			if fl, ok := handleInt(val[i], rules); ok {
				ret = append(ret, fl)
				return true
			}
			return false
		})
}

func handleFloatS(val []string, rules map[string]interface{}) ([]float64, bool) {
	ret := []float64{}
	return ret, allOk(val, rules,
		func(i int) bool {
			if fl, ok := handleFloat(val[i], rules); ok {
				ret = append(ret, fl)
				return true
			}
			return false
		})
}

func handleBoolS(val []string, rules map[string]interface{}) ([]bool, bool) {
	ret := []bool{}
	return ret, allOk(val, rules,
		func(i int) bool {
			if b, ok := handleBool(val[i], rules); ok {
				ret = append(ret, b)
				return true
			}
			return false
		})
}

func isNum(i interface{}) bool {
	_, in := i.(int)
	_, fl := i.(float64)
	return in || fl
}

func (r *Rules) Extract(dat map[string][]string) (map[string]interface{}, error) {
	return r.extract(dat, nil)
}

// field: 1									// Field will be in ret without check if it exists in input.
// field: false								// Field will be entirely ignored, exists or not in input.
// field: "must"							// Field is mandatory, error occurs when not exists in input. Field is handled as string.
// field: {"type":"number"}					// Field is handled as number, if it exists in input. If not a number, error will be raised. *
// field: {"type":"number", "must": true}	// Field is handled as number, if i exists in input, error will be raised if not exists. If not a number, error will be raised.	*
//
// Generally, a field must satisfy all requirements to pass, otherwise an error will be raised.
// Only exception if a field does not exist. Then, it will be simply left from ret out unless "must" is specified.
// If "must" is specified and field does not exist, an error will be raised.
func (r *Rules) extract(dat map[string][]string, unknown_type_handler func(val []string, rules map[string]interface{}) (interface{}, error)) (map[string]interface{}, error) {
	ret := map[string]interface{}{}
	// missing := false
	for i, v := range r.R {
		val, hasval := dat[i]
		isnum := isNum(v)
		if isnum { // Without any check
			if hasval {
				ret[i] = val[0]
			}
		} else if str, is_str := v.(string); is_str && str == "must" {
			if !hasval {
				return ret, fmt.Errorf("Mandatory field \"%s\" is missing.", i)
			}
			ret[i] = val[0]
		} else if obj, is_obj := v.(map[string]interface{}); is_obj {
			_, must := obj["must"]
			if must && !hasval {
				return ret, fmt.Errorf("Mandatory field \"%s\" is missing.", i)
			} else if !hasval || len(val) == 0 {
				continue
			}
			typ, hastype := obj["type"]
			if !hastype {
				if len(val) > 1 {
					return ret, fmt.Errorf("Typeless (string) field \"%s\" sent with multiple values.", i)
				}
				s, passed := handleString(val[0], obj)
				if passed {
					ret[i] = s
				} else if must {
					return ret, fmt.Errorf("Typeless (string) field \"%s\" not passed.", i)
				}
			} else {
				// passed := false
				switch typ {
				case "bools":
					s, pass := handleBoolS(val, obj)
					if !pass {
						return ret, fmt.Errorf(slice_err, i)
					} else {
						ret[i] = s
					}
				case "strings":
					s, pass := handleStringS(val, obj)
					if !pass {
						return ret, fmt.Errorf(slice_err, i)
					} else {
						ret[i] = s
					}
				case "ints":
					s, pass := handleIntS(val, obj)
					if !pass {
						return ret, fmt.Errorf(slice_err, i)
					} else {
						ret[i] = s
					}
				case "floats":
					s, pass := handleFloatS(val, obj)
					if !pass {
						return ret, fmt.Errorf(slice_err, i)
					} else {
						ret[i] = s
					}
				default:
					if len(val) > 1 {
						return ret, fmt.Errorf("Field \"%s\" sent with multiple values.", i)
					}
					switch typ {
					case "bool":
						s, pass := handleBool(val[0], obj)
						if !pass {
							return ret, fmt.Errorf(single_err, i)
						} else {
							ret[i] = s
						}
					case "string":
						s, pass := handleString(val[0], obj)
						if !pass {
							return ret, fmt.Errorf(single_err, i)
						} else {
							ret[i] = s
						}
					case "int":
						s, pass := handleInt(val[0], obj)
						if !pass {
							return ret, fmt.Errorf(single_err, i)
						} else {
							ret[i] = s
						}
					case "float":
						s, pass := handleFloat(val[0], obj)
						if !pass {
							return ret, fmt.Errorf(single_err, i)
						} else {
							ret[i] = s
						}
					default:
						if unknown_type_handler == nil {
							return ret, fmt.Errorf("Field \"%s\" has unknown type.", i)
						} else {
							s, err := unknown_type_handler(val, obj)
							if err != nil {
								return ret, fmt.Errorf("Outside field \"%v\" not passed: %v", i, err.Error())
							} else {
								ret[i] = s
							}
						}
					}
				}
			}
		} else if boo, is_bool := v.(bool); is_bool && boo == false {
			continue
		} else {
			return nil, fmt.Errorf("Can't interpret rule command.")
		}
	}
	return ret, nil
}

func (r *Rules) ResetRules(templ map[string]interface{}) {
	r.R = templ
}

func New(templ map[string]interface{}) *Rules {
	r := &Rules{templ}
	return r
}
