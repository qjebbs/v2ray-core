package link

import (
	"github.com/v2fly/v2ray-core/v4/common"
	"github.com/v2fly/v2ray-core/v4/common/errors"
)

func init() {
	common.Must(RegisterParser(&Parser{
		Name:   "Official",
		Scheme: []string{"vmess"},
		Parse:  ParseVmess,
	}))
}

// ParseVmess parses official vemss link to Link
func ParseVmess(vmess string) (Link, error) {
	// TODO: Official vmess:// parse support
	return nil, errors.New("not implemented")
}
