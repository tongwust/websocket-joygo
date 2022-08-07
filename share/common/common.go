package common

func InSlice(items []string,item string) bool {
	for _, one := range items{
		if one == item{
			return true
		}
	}
	return false
}

func ClearRepSlice(slc []string) []string {
	result := make([]string,0,1000)
	temp := make(map[string]byte,0)
	for _,s := range slc{
		if len(s) > 0{
			l := len(temp)
			temp[s] = 0
			if len(temp) != l{
				result = append(result,s)
			}
		}
	}
	return result
}