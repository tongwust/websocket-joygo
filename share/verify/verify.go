package verify

import (
	"crypto/md5"
	"encoding/hex"
	"sort"
	"strings"
)

const SALT = "u9$1n#.(n3w9dnfia"

func MD5(s string) string  {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func Verify(mp map[string]string,token string) bool {
	var keys []string
	for k := range mp{
		keys = append(keys,k)
	}
	sort.Strings(keys)
	var vals []string
	for _,k := range keys{
		vals = append(vals,k + "=" + mp[k])
	}
	if MD5(strings.Join(vals,"&") + "&salt=" + SALT) == token{
		return true
	}
	return false
}