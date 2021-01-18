package router

import (
	"strings"
	"time"

	"v2ray.com/core/common/net"
	"v2ray.com/core/features/outbound"
	"v2ray.com/core/features/routing"
)

// CIDRList is an alias of []*CIDR to provide sort.Interface.
type CIDRList []*CIDR

// Len implements sort.Interface.
func (l *CIDRList) Len() int {
	return len(*l)
}

// Less implements sort.Interface.
func (l *CIDRList) Less(i int, j int) bool {
	ci := (*l)[i]
	cj := (*l)[j]

	if len(ci.Ip) < len(cj.Ip) {
		return true
	}

	if len(ci.Ip) > len(cj.Ip) {
		return false
	}

	for k := 0; k < len(ci.Ip); k++ {
		if ci.Ip[k] < cj.Ip[k] {
			return true
		}

		if ci.Ip[k] > cj.Ip[k] {
			return false
		}
	}

	return ci.Prefix < cj.Prefix
}

// Swap implements sort.Interface.
func (l *CIDRList) Swap(i int, j int) {
	(*l)[i], (*l)[j] = (*l)[j], (*l)[i]
}

type Rule struct {
	Tag       string
	Balancer  *Balancer
	Condition Condition
}

func (r *Rule) GetTag() (string, error) {
	if r.Balancer != nil {
		return r.Balancer.PickOutbound()
	}
	return r.Tag, nil
}

// Apply checks rule matching of current routing context.
func (r *Rule) Apply(ctx routing.Context) bool {
	return r.Condition.Apply(ctx)
}

func (rr *RoutingRule) BuildCondition() (Condition, error) {
	conds := NewConditionChan()

	if len(rr.Domain) > 0 {
		matcher, err := NewDomainMatcher(rr.Domain)
		if err != nil {
			return nil, newError("failed to build domain condition").Base(err)
		}
		conds.Add(matcher)
	}

	if len(rr.UserEmail) > 0 {
		conds.Add(NewUserMatcher(rr.UserEmail))
	}

	if len(rr.InboundTag) > 0 {
		conds.Add(NewInboundTagMatcher(rr.InboundTag))
	}

	if rr.PortList != nil {
		conds.Add(NewPortMatcher(rr.PortList, false))
	} else if rr.PortRange != nil {
		conds.Add(NewPortMatcher(&net.PortList{Range: []*net.PortRange{rr.PortRange}}, false))
	}

	if rr.SourcePortList != nil {
		conds.Add(NewPortMatcher(rr.SourcePortList, true))
	}

	if len(rr.Networks) > 0 {
		conds.Add(NewNetworkMatcher(rr.Networks))
	} else if rr.NetworkList != nil {
		conds.Add(NewNetworkMatcher(rr.NetworkList.Network))
	}

	if len(rr.Geoip) > 0 {
		cond, err := NewMultiGeoIPMatcher(rr.Geoip, false)
		if err != nil {
			return nil, err
		}
		conds.Add(cond)
	} else if len(rr.Cidr) > 0 {
		cond, err := NewMultiGeoIPMatcher([]*GeoIP{{Cidr: rr.Cidr}}, false)
		if err != nil {
			return nil, err
		}
		conds.Add(cond)
	}

	if len(rr.SourceGeoip) > 0 {
		cond, err := NewMultiGeoIPMatcher(rr.SourceGeoip, true)
		if err != nil {
			return nil, err
		}
		conds.Add(cond)
	} else if len(rr.SourceCidr) > 0 {
		cond, err := NewMultiGeoIPMatcher([]*GeoIP{{Cidr: rr.SourceCidr}}, true)
		if err != nil {
			return nil, err
		}
		conds.Add(cond)
	}

	if len(rr.Protocol) > 0 {
		conds.Add(NewProtocolMatcher(rr.Protocol))
	}

	if len(rr.Attributes) > 0 {
		cond, err := NewAttributeMatcher(rr.Attributes)
		if err != nil {
			return nil, err
		}
		conds.Add(cond)
	}

	if conds.Len() == 0 {
		return nil, newError("this rule has no effective fields").AtWarning()
	}

	return conds, nil
}

// Build builds the balancing rule
func (br *BalancingRule) Build(ohm outbound.Manager, dispatcher routing.Dispatcher) (*Balancer, error) {
	var strategy routing.BalancingStrategy
	switch br.Strategy {
	case BalancingRule_LeastLoad:
		i, err := br.StrategySettings.GetInstance()
		if err != nil {
			return nil, err
		}
		s, ok := i.(*StrategyLeastLoadConfig)
		if !ok {
			return nil, newError("not a StrategyLeastLoadConfig").AtError()
		}
		strategy = &LeastLoadStrategy{
			settings:   s,
			HealthPing: healthPingFromConfig(s.HealthCheck, dispatcher),
		}
	case BalancingRule_Random:
		fallthrough
	default:
		strategy = &RandomStrategy{}
	}
	return &Balancer{
		selectors:   br.OutboundSelector,
		ohm:         ohm,
		fallbackTag: br.FallbackTag,
		strategy:    strategy,
	}, nil
}

func healthPingFromConfig(config *HealthPingConfig, dispatcher routing.Dispatcher) *HealthPing {
	settings := &HealthPingSettings{}
	if config != nil {
		settings = &HealthPingSettings{
			Destination: strings.TrimSpace(config.Destination),
			Interval:    time.Duration(config.Interval),
			Rounds:      int(config.Rounds),
			Timeout:     time.Duration(config.Timeout),
		}
	}
	if settings.Destination == "" {
		settings.Destination = "http://www.google.com/gen_204"
	}
	if settings.Interval == 0 {
		settings.Interval = time.Duration(15) * time.Minute
	} else if settings.Interval < 10 {
		newError("health check interval is too small, 10s is applied").AtWarning().WriteToLog()
		settings.Interval = time.Duration(10) * time.Second
	}
	if settings.Rounds <= 0 {
		settings.Rounds = 1
	}
	if settings.Timeout <= 0 {
		// results are saved after all health pings finish,
		// a larger timeout could possibly makes checks run longer
		settings.Timeout = time.Duration(5) * time.Second
	}
	return &HealthPing{
		dispatcher: dispatcher,
		Settings:   settings,
		Results:    nil,
	}
}
