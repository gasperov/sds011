package main

import (
	"bytes"
	"fmt"
)

type SDS011 struct {
	PM10 float64
	PM25 float64

	len         int
	pm10_serial int
	pm25_serial int
	checksum    int

	dbg []byte
}

func NewSDS011() SDS011 {
	ret := SDS011{}
	ret.dbg = []byte{}
	return ret
}

func (s *SDS011) Read(b byte) bool {
	s.dbg = append(s.dbg, b)
	if len(s.dbg) > 12 {
		s.dbg = append([]byte{}, s.dbg[1:]...)
	}
	value := int(b)
	switch s.len {
	case (0):
		if value != 170 {
			s.len = -1
		}
		break
	case (1):
		if value != 192 {
			s.len = -1
		}
		break
	case (2):
		s.pm25_serial = value
		s.checksum = value
		break
	case (3):
		s.pm25_serial += (value << 8)
		s.checksum += value
		break
	case (4):
		s.pm10_serial = value
		s.checksum += value
		break
	case (5):
		s.pm10_serial += (value << 8)
		s.checksum += value
		break
	case (6):
		s.checksum += value
		break
	case (7):
		s.checksum += value
		break
	case (8):
		if value != (s.checksum % 256) {
			s.len = -1
		}
		break
	case (9):
		if value != 171 {
			s.len = -1
		}
		break
	}
	if s.len == -1 {
		fmt.Printf("buf error %v\n", s.dbg)
	}
	s.len++
	if s.len == 10 {
		s.PM10 = float64(s.pm10_serial) / 10.0
		s.PM25 = float64(s.pm25_serial) / 10.0
		s.len = 0
		s.pm10_serial = 0.0
		s.pm25_serial = 0.0
		s.checksum = 0
		s.dbg = []byte{}
		//fmt.Printf("have %v, %v\n", s.PM10, s.PM25)
		return true
	}

	return false
}

func (s *SDS011) ReadBytes(b []byte) bool {
	ret := false
	for i := range b {
		if s.Read(b[i]) {
			ret = true
		}
	}
	return ret
}

func csum(b []byte) byte {
	var csum int
	for _, v := range b {
		csum += int(v)
	}
	return byte(csum & 0xff)
}

func (s *SDS011) SetPeriod(minutes int) []byte {
	if minutes < 0 || minutes > 30 {
		panic("period must be in [0, 30]")
	}

	data := make([]byte, 13)
	copy(data, []byte{0x08, 0x01, byte(minutes)})
	data = append(data, []byte{0xff, 0xff}...)

	var buf bytes.Buffer
	buf.Write([]byte{0xaa, 0xb4})
	buf.Write(data)
	buf.WriteByte(csum(data))
	buf.WriteByte(0xab)

	return buf.Bytes()
}
