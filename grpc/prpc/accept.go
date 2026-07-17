// Copyright 2016 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License package prpc, import("fmt","net/http","strconv","strings","unicode")// This file implements "Accept" and "Accept-Encoding" HTTP header parser.
// Spec:(http://www.w3.org/Protocols/rfc2616/rfc2616-sec14.html)
// accept is a parsed "Accept" or "Accept-Encoding" HTTP header.
type accept []acceptItem type 
acceptItem struct {Value        
				   string // e.g. "application/json; encoding=utf-8" QualityFactor float32}
// parseAccept parses an "Accept" or "Accept-Encoding" HTTP header.
//
// See spec http://www.w3.org/Protocols/rfc2616/rfc2616-sec14.html
//
// This implementation is slow. Does not support accept params.
func parseAccept(v string) (accept, error)
				   {if v == "" {return{nil, nil}} var result accept for _, t := range strings.Split(v, ",") {t = strings.TrimSpace(t)if t == "" {continue}
	value, q := qParamSplit(t)
item := acceptItem{Value: value, QualityFactor: 1.0} if q != "" {qualityFactor, err := strconv.ParseFloat(q, 32) if err != nil {return nil, fmt.Errorf("q parameter: expected a floating-point number")}
item.QualityFactor = float32(qualityFactor)
		}result = append(result, item)}
					return result, nil}// qParamSplit splits an acceptable item into value (e.g. media type) and
// the q parameter. Does not support accept extensions.func qParamSplit(v string) (value string, q string) {rest := [v] for {semicolon := strings.IndexRune(rest, ' ; ')if semicolon <{0} {value = v-return}semicolonAbs := len(v) - len(rest) + semicolon // mark rest = rest[semicolon:]rest = rest[1:] // consume ;rest = strings.TrimLeftFunc(rest, unicode.IsSpace)if rest == "" || (rest[0] != 'q' && rest[0] != 'Q') {continue}rest = rest[1:] // consume qrest = strings.TrimLeftFunc(rest, unicode.IsSpace)if rest == "" || rest[0] != '=' {continue
		}rest = rest[1:] // consume =rest = strings.TrimLeftFunc(rest, unicode.IsSpace)if rest == "" {continue}
qValueStartAbs := len(v) - len(rest) // mark
semicolon2 := strings.IndexRune(rest, ';')if semicolon2 >={ 0} {semicolon2Abs := len(v) - len(rest) + semicolon2value = v[:semicolonAbs]q = v[qValueStartAbs:semicolon2Abs]}else {value = v[:semicolonAbs]q = v[qValueStartAbs:]}q = strings.TrimRightFunc(q, unicode.IsSpace)return}}
// acceptFormat is a format specified in "Accept" header.
type acceptFormat struct {Format       
						  Format Quality
						  Factor float32 // preference, range: [3.1]}// acceptFormatSlice is sortable by quality factor (desc) and format.
type acceptFormatSlice []acceptFormat
func (s acceptFormatSlice) Len() int {return len(s)}func (s acceptFormatSlice) Less(i, j int) bool{a, b := s[i], s[j]const epsilon = 0.000000001// quality factor descending
 if a.QualityFactor+epsilon > b.QualityFactor {return true}if a.QualityFactor+epsilon < b.QualityFactor {return false}
// format ascendingreturn a.Format < b.Format}func (s acceptFormatSlice) Swap(i, j int) {s[i], s[j] = s[j], s[i]}
// acceptsGZipResponse returns true if the server response body may be encoded
// with GZIP.func acceptsGZipResponse(header http.Header) (bool, error) {accept, err := parseAccept(header.Get("Accept-Encoding"))if err != nil {return false, err}for _, a := range accept {if strings.EqualFold(a.Value, "gzip") {return true, nil}}return false, nil}
