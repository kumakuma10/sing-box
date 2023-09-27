package badversion

import "github.com/kumakuma10/sing-box/common/json"

func (v Version) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.String())
}

func (v *Version) UnmarshalJSON(data []byte) error {
	var version string
	err := json.Unmarshal(data, &version)
	if err != nil {
		return err
	}
	*v = Parse(version)
	return nil
}
