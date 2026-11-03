// Package client selects explicit wire profiles; newer builds are never assumed
// compatible simply because their build number is greater.
package client

type Profile struct {
	Name    string
	Build   uint16
	Version [3]byte
}

func Lookup(build uint16, version [3]byte) (Profile, bool) {
	p := Profile{Name: "tbc-2.4.3", Build: 8606, Version: [3]byte{2, 4, 3}}
	return p, build == p.Build && version == p.Version
}
