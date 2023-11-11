package route

import (
	"errors"

	"github.com/kumakuma10/sing-box/adapter"
)

func (r *Router) AddInbound(inbound adapter.Inbound) error {
	r.actionLock.Lock()
	defer r.actionLock.Unlock()
	if _, ok := r.inboundByTag[inbound.Tag()]; ok {
		return errors.New("the inbound is exist")
	}
	r.inboundByTag[inbound.Tag()] = inbound
	return nil
}

func (r *Router) DelInbound(tag string) error {
	r.actionLock.Lock()
	defer r.actionLock.Unlock()
	if _, ok := r.inboundByTag[tag]; ok {
		delete(r.inboundByTag, tag)
	} else {
		return errors.New("the inbound not have")
	}
	return nil
}

// func (r *Router) UpdateDnsRules(rules []option.DNSRule) error {
// 	dnsRules := make([]adapter.DNSRule, 0, len(rules))
// 	for i, rule := range rules {
// 		dnsRule, err := NewDNSRule(r, r.logger, rule)
// 		if err != nil {
// 			return E.Cause(err, "parse dns rule[", i, "]")
// 		}
// 		err = dnsRule.Start()
// 		if err != nil {
// 			return E.Cause(err, "initialize DNS rule[", i, "]")
// 		}
// 		dnsRules = append(dnsRules, dnsRule)
// 	}
// 	var tempRules []adapter.DNSRule
// 	r.actionLock.Lock()
// 	r.dnsRules = tempRules
// 	r.dnsRules = dnsRules
// 	r.actionLock.Unlock()
// 	for i, rule := range tempRules {
// 		err := rule.Close()
// 		if err != nil {
// 			return E.Cause(err, "closing DNS rule[", i, "]")
// 		}
// 	}
// 	return nil
// }
