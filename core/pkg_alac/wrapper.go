package alac

import (
	"encoding/binary"
	"fmt"
)

type Decoder struct {
	alac *Alac
}

func New(cookie []byte) (*Decoder, error) {
	if len(cookie) < 24 {
		return nil, fmt.Errorf("invalid cookie length: %d", len(cookie))
	}

	maxSamples := binary.BigEndian.Uint32(cookie[0:4])
	sampleSize := int(cookie[5])
	channels := int(cookie[9])

	a := create_alac(sampleSize, channels)
	a.setinfo_max_samples_per_frame = maxSamples
	a.setinfo_7a = cookie[4]
	a.setinfo_sample_size = cookie[5]
	a.setinfo_rice_historymult = cookie[6]
	a.setinfo_rice_initialhistory = cookie[7]
	a.setinfo_rice_kmodifier = cookie[8]
	a.setinfo_7f = cookie[9]
	a.setinfo_80 = binary.BigEndian.Uint16(cookie[10:12])
	a.setinfo_82 = binary.BigEndian.Uint32(cookie[12:16])
	a.setinfo_86 = binary.BigEndian.Uint32(cookie[16:20])
	a.setinfo_8a_rate = binary.BigEndian.Uint32(cookie[20:24])

	a.allocateBuffers()

	return &Decoder{alac: a}, nil
}

func (d *Decoder) Decode(in []byte) ([]byte, error) {
	if d.alac == nil {
		return nil, fmt.Errorf("decoder not initialized")
	}
	out := d.alac.decodeFrame(in)
	return out, nil
}
