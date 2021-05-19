// Copyright 2020 Google LLC. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package crypto

const (
	// TestVendorRSAPriv stores a TEST/DEMO key for signing firmware metadata.
	TestVendorRSAPriv = `
-----BEGIN RSA PRIVATE KEY-----
MIIEpQIBAAKCAQEAumH27frnBBnP/sgSbgiGkkxUGflZOJU0AZ+zDlGh4xIsam+8
mSI/DKGodwQ6rDtjpWhLmIAPEusRC0Pw78+zxGZihxNEmuHM4vEQkwBLKAPAX8oo
hE5cZADINwasFloO0hgynqvUJs/qpmp+FnxDQQe/ISsmsFDdQ+8NyuJuX203rMC3
yVE+s7wazBEvdZkOkb38UkuiWcYplrWsjr1eHjNt2P0s1e46uUUTbDDp1YxSM8Jl
fk/62/txofBGGIr7GqiR2g3KoiXz42PW40MpKpdzHwhwWpZ8sPU0p9GQT0rEtZDK
Sr67roVq+YMWFjTE6hPTjvYa+oPEviHJgDh3FwIDAQABAoIBAQCTzUonUJfQXbPe
xMQws9wbuiHbcyw4vcE/JGi3Cy9IxcmpIjC6czjyiGPy9cyddn8/1WRVbAAILZcX
iupPEjPppJOvsCzwce2rbiWJnWr8MXVlwQe+W/HSb/hWApmHJCWqn/vMblDP8oOP
MtYIeMRQlpcP84s7uPSuga07XbLPQo8capaSOrOEYMUjzuTFK42doBSm7BGwLD0/
xyA3sJs5m3tpKEtuwhOTQ+lZNT36bLIzkHgWcvqAVjUP8yoHjtAMiqZOF1M0fmCu
KUGiak1hmlEw/afdl5AFkZIIjRs+uTqjuIeh+EBsHaPZCX7RnlR3hVYmoixGDx2E
u4WvZdExAoGBAOfRPkWhnRFzUUXr3AHqy+ETKEhttVEHdEVy0h+wpTaWCVu/75Ym
C1dgL5cYUiaKZojKHMYGS6R5nNv3XtuqDuPURY6l99VwAgfTXlOHkKHl59aG/vfh
h5EuJkAVM3P38JGRuAc4Ke4wRFxQ7P8eHnoL004nuVl7qqNAa8XIGhrPAoGBAM3T
X6KbPNhR93OQNQ6McK/DDiNCx7h945/gxxByc4EWVbzv47ELfURFkL8Xdj2q3Oa0
jPiMPfcDifglwwOTqmaI3f8OpDgUYCi+/zlOEqbkfvhezfop5zHx1qgI4EkE76C1
1mQUnb7+orT8SgQZArO4UblEdZwzcRWwhUqIg1E5AoGBALbBpJlaryx5wGIibwFn
7THVW5W2QBLQkJ7LxdJL/gQJxvj5WVYDSj+pSfuRpfpSdEF1LbgEcJALfFmCLNt6
t2BwEiJCwB8ZvdATmDK8Fo88ZEkhhlNADxOq0WcGD9lmZ9crjWzLn2rzzIEHH8CF
KzvGpODhumNMdptbh1uWxNXLAoGBAJjQwQann2s0oDaK8PmWR+wXNB317PcLcL85
UlLhxuQmww1+Rl0inKTdyXQ3ZTCv9UbM8oVvCmqllABLeRjkv/VU1Q4TvtVsO2DF
PhU75BlJOQQKz39XMTIhzjAANxW/tnJpz32K2Pv/bqpVTlbwgtMQnIyjSXbpsqJZ
5vzJkkXxAoGAfYGjwrk8eb5+gOpCTAzkXFnbglomHq7eP6uX+rfOEkmsDHPl8u1o
cxj/AZRxN9Xdvc13CuKYQ6m7l23fpLl6fd59/pviYUopLvqhtfCE8khb0UBc2c3/
nJaDjwwm2fmeALKdhqOejIGkKSiVbc0OJbCzPMUt0LohVodSds984rk=
-----END RSA PRIVATE KEY-----`

	// TestVendorRSAPub stores a TEST/DEMO key for verifying signatures on firmware metadata.
	TestVendorRSAPub = `
-----BEGIN RSA PUBLIC KEY-----
MIIBCgKCAQEAumH27frnBBnP/sgSbgiGkkxUGflZOJU0AZ+zDlGh4xIsam+8mSI/
DKGodwQ6rDtjpWhLmIAPEusRC0Pw78+zxGZihxNEmuHM4vEQkwBLKAPAX8oohE5c
ZADINwasFloO0hgynqvUJs/qpmp+FnxDQQe/ISsmsFDdQ+8NyuJuX203rMC3yVE+
s7wazBEvdZkOkb38UkuiWcYplrWsjr1eHjNt2P0s1e46uUUTbDDp1YxSM8Jlfk/6
2/txofBGGIr7GqiR2g3KoiXz42PW40MpKpdzHwhwWpZ8sPU0p9GQT0rEtZDKSr67
roVq+YMWFjTE6hPTjvYa+oPEviHJgDh3FwIDAQAB
-----END RSA PUBLIC KEY-----`

	// TestFTPersonalityPriv stores a TEST/DEMO key used to sign the personality checkpoints.
	TestFTPersonalityPriv = "PRIVATE+KEY+ft_personality+e8a242bd+ATWzA37YLiAsXuDHcPJtQRUye7xPhKGIUrZuNznGZDox"

	// TestFTPersonalityPub is the TEST/DEMO key used to verify signatures on the personality checkpoints.
	TestFTPersonalityPub = "ft_personality+e8a242bd+Aet8wMj2c6gk0hN/Ah7EfkSJWXWRg1JizEjkPnAWYpLY"

	// TestAnnotationPriv is the TEST/DEMO key used to signed annotations.
	TestAnnotationPriv = `-----BEGIN RSA PRIVATE KEY-----
MIIEogIBAAKCAQEAoGKwBzNMxdPS1Uo+BAykf2C9nuLpLkXBSpINYOiGcJeBpV04
MUw7BrW0ynvwwQL1h+yVWfiRL3xMDQmMkr/EjPEiW4VXji/lVMDClIi4mlMdygiG
hM1Na5PpCAg8+khpFOnSlpC95EdGoU3c+iezTJH0nHH6KUfT6cMZMLNdjI5MtES3
aZET4/iQRkmqv+FXP4M5QmQb1PE7iEGZ6J3IUQKJrpqilz0RZM535Tz89cchD1Mh
cIwi3MO45dbZP1/0lDDYinK/VnklYlKuZjXTC29TWFzStF34YPDA1dFaGZug2/oC
jdMLyQBEZJsEYEwF6jhdp4p7zHOSQ5Xc68xbDQIDAQABAoIBADUsgu/gMjPkZqIQ
Wz88cc1JZZSn5mdQ+SSgB495iBkMIg+ROHAftfIjjC0VqlxTftPxvBJ4Nqpnq08n
O1PsAF46FAoDy2N4va+7uMdGDO4dYGL7MJ4W8vQXtcrT8GOKXkxwuUDx/AMTHnec
OQc24lsgiNjVcPr+tWNrK47Z6MoQXT7CaZpQMaZkYxj8T3g63Anfdjoichv4jByS
/TQzGElqea1IxzUpvGXMf3n6IV4GxlVdDX0rFXLM9vp7k5Av8vBx4YCnTi48u0m0
JbdAzfgeOla8GkWzzgYSkAY0CMhiSNB6eUkbO9qxHH0o0JZKuU8kP1bci/LBhcCL
MyeGSAECgYEAzTrYxne3hUeVxAmXABtbgWtZbmlfE3FKXf+Jy4UOaLa/KZeHAaIK
e32KjYTITfNlNZ+g6nYAy1hI7kJvutnznjY3Nfv9gtRNmK96ej+1SabMD68raDGY
7/YR5mYr7KcI9Am5HFKTzDKBHUry0OB3v5JOPMrr9Le/LZ6pxQzKOT0CgYEAyA/b
0bCylSGX2LW63IuEdyPEv5zIL/7cTsUNdtWbSofBpG3DdT9Fm2AvfnipJRxNIcX+
t5zAqPrWSx23ovhr7GsdPawjn8oNpwO5GyHmpiFiMtMnykEAXtQNuJbPGcpKFWnH
Ydp8pSBVyK0C5vlJWINNHYa0vTEzKW1MwbBVphECgYAu8PzQOGXDmGILGt5s6dT+
Px2PgY57lfgak+5inKZ1EQecbco1d2jKYiakw/BE1B0cLMzTk/YOjLzxskR4Co4M
a/4o3OBZYlH1UH3FJHlExV/7XmehR2bhy/jAKDJ3yKTlnKu4bLLdi9e4aYIsgIsj
SEWY5hkeOkECID5YkdpXSQKBgAuG3mN2itOM2/LghaOvZjJ3HR7tKZuaU5c2Q1BV
fl0M9VtD978JpjkNka73xMcemlMX1VU+8trJmQ865xm8tnsosMac5HCQc7jrvf6S
NXfc9It5HxHILP1JuoCoL8aMoTgaoCJDNGtPMaIeVcx5EIDJD+hjmoZMD2aTpZiD
UGwBAoGAIDPx1CWMWqwLIF66BfbxnYC7iPKWc0oTUKFx45yxjgwegzj5n9VCmJw0
nSEBOasrfQKsWg0gbtgoxxg6awY12czAWRukp5zyoTT+PSGi32gHepCOan4MqJ5w
QeDeXVCJOme4xiBEhnC95flKCsfN9yBMwFy2N7qj0T1DbyKZtSc=
-----END RSA PRIVATE KEY-----`

	// TestAnnotationPub is the TEST/DEMO key used to verify annotation signatures.
	TestAnnotationPub = `-----BEGIN RSA PUBLIC KEY-----
MIIBCgKCAQEAoGKwBzNMxdPS1Uo+BAykf2C9nuLpLkXBSpINYOiGcJeBpV04MUw7
BrW0ynvwwQL1h+yVWfiRL3xMDQmMkr/EjPEiW4VXji/lVMDClIi4mlMdygiGhM1N
a5PpCAg8+khpFOnSlpC95EdGoU3c+iezTJH0nHH6KUfT6cMZMLNdjI5MtES3aZET
4/iQRkmqv+FXP4M5QmQb1PE7iEGZ6J3IUQKJrpqilz0RZM535Tz89cchD1MhcIwi
3MO45dbZP1/0lDDYinK/VnklYlKuZjXTC29TWFzStF34YPDA1dFaGZug2/oCjdML
yQBEZJsEYEwF6jhdp4p7zHOSQ5Xc68xbDQIDAQAB
-----END RSA PUBLIC KEY-----`
)
