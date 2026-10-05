// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package naming

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExported(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		parts []string
		want  string
	}{
		{name: "Lower camel case", parts: []string{"fooBar"}, want: "FooBar"},
		{name: "Upper camel case is unchanged", parts: []string{"FooBar"}, want: "FooBar"},
		{name: "Single lower-case word", parts: []string{"pet"}, want: "Pet"},
		{name: "Snake case", parts: []string{"client_address"}, want: "ClientAddress"},
		{name: "Screaming snake case", parts: []string{"CLIENT_ADDRESS"}, want: "ClientAddress"},
		{name: "Kebab case", parts: []string{"client-address"}, want: "ClientAddress"},
		{name: "Dotted name", parts: []string{"client.address"}, want: "ClientAddress"},
		{name: "Slashes", parts: []string{"pets/{id}/owner"}, want: "PetsIDOwner"},
		{name: "Spaces", parts: []string{"client address"}, want: "ClientAddress"},
		{name: "Colon", parts: []string{"geo:lat"}, want: "GeoLat"},
		{name: "Tilde", parts: []string{"a~b"}, want: "AB"},
		{name: "Brackets", parts: []string{"filter[name]"}, want: "FilterName"},
		{name: "Array marker", parts: []string{"ids[]"}, want: "IDs"},
		{name: "Parentheses", parts: []string{"size(cm)"}, want: "SizeCm"},
		{name: "Mixed separators", parts: []string{"a_b-c.d e"}, want: "ABCDE"},
		{name: "Repeated separators", parts: []string{"a__b--c"}, want: "ABC"},
		{name: "Leading and trailing separators", parts: []string{"_name_"}, want: "Name"},
		{name: "Leading underscore", parts: []string{"_links"}, want: "Links"},
		{name: "Upper run before a word", parts: []string{"HTTPServer"}, want: "HTTPServer"},
		{name: "Upper run after a word", parts: []string{"getHTTPResponseCode"}, want: "GetHTTPResponseCode"},
		{name: "Upper run at the end", parts: []string{"parseJSON"}, want: "ParseJSON"},
		{name: "Initialism in lower case", parts: []string{"url"}, want: "URL"},
		{name: "Initialism in title case", parts: []string{"userId"}, want: "UserID"},
		{name: "Initialism in snake case", parts: []string{"user_id"}, want: "UserID"},
		{name: "Initialism in all caps", parts: []string{"USER_ID"}, want: "UserID"},
		{name: "Mixed-case initialism is rewritten", parts: []string{"XMLHttpRequest"}, want: "XMLHTTPRequest"},
		{name: "HTTPS is its own initialism", parts: []string{"https_url"}, want: "HTTPSURL"},
		{name: "Initialism only matches a whole word", parts: []string{"idle"}, want: "Idle"},
		{name: "Initialism inside a lower-case word is left alone", parts: []string{"userid"}, want: "Userid"},
		{name: "Plural initialism in camel case", parts: []string{"userIDs"}, want: "UserIDs"},
		{name: "Plural initialism in lower case", parts: []string{"urls"}, want: "URLs"},
		{name: "Plural initialism in all caps", parts: []string{"USER_IDS"}, want: "UserIDs"},
		{name: "Plural initialism before a word", parts: []string{"IDsList"}, want: "IDsList"},
		{name: "Plural initialism with digits", parts: []string{"IDs2"}, want: "IDs2"},
		{name: "Upper run with s that is no initialism splits as usual", parts: []string{"ABs"}, want: "ABs"},
		{name: "Initialism with trailing digits", parts: []string{"id2"}, want: "ID2"},
		{name: "Initialism that ends in a digit", parts: []string{"utf8_string"}, want: "UTF8String"},
		{name: "Initialism with a digit before a capital", parts: []string{"UTF8String"}, want: "UTF8String"},
		{name: "Letters and digits that are no initialism", parts: []string{"utf16"}, want: "Utf16"},
		{name: "Every default initialism", parts: []string{"api ascii cpu css dns eof html http ip json oas rpc sql ssh tcp tls ttl udp uri uuid xml"}, want: "APIASCIICPUCSSDNSEOFHTMLHTTPIPJSONOASRPCSQLSSHTCPTLSTTLUDPURIUUIDXML"},
		{name: "All-caps word that is no initialism", parts: []string{"SSN"}, want: "Ssn"},
		{name: "All-caps enum value", parts: []string{"ACTIVE"}, want: "Active"},
		{name: "Single capital letters", parts: []string{"iPhone"}, want: "IPhone"},
		{name: "Capital inside a word", parts: []string{"McDonald"}, want: "McDonald"},
		{name: "Initialism prefix of a word splits before the last capital", parts: []string{"OAuth2Token"}, want: "OAuth2Token"},
		{name: "Digits stay with the word before", parts: []string{"v2"}, want: "V2"},
		{name: "Letters after digits start a word", parts: []string{"v2api"}, want: "V2API"},
		{name: "Capitals after digits start a word", parts: []string{"V2API"}, want: "V2API"},
		{name: "Digits inside camel case", parts: []string{"s3Bucket"}, want: "S3Bucket"},
		{name: "Digits after an upper run", parts: []string{"HTTP2server"}, want: "HTTP2Server"},
		{name: "Separated digits", parts: []string{"item_2"}, want: "Item2"},
		{name: "Digits only", parts: []string{"200"}, want: "N200"},
		{name: "Leading digit gets N", parts: []string{"1st"}, want: "N1st"},
		{name: "Leading digits before capitals", parts: []string{"2FA"}, want: "N2Fa"},
		{name: "Leading digits before camel case", parts: []string{"404NotFound"}, want: "N404NotFound"},
		{name: "Leading digit after a separator", parts: []string{"_3d"}, want: "N3d"},
		{name: "Decimal point between digits", parts: []string{"1.5"}, want: "N1Dot5"},
		{name: "Version number", parts: []string{"v1.2.3"}, want: "V1Dot2Dot3"},
		{name: "Dot after a letter is a separator", parts: []string{"a.1"}, want: "A1"},
		{name: "Leading minus", parts: []string{"-1"}, want: "Minus1"},
		{name: "Leading minus on a sort field", parts: []string{"-created_at"}, want: "MinusCreatedAt"},
		{name: "Minus inside a name is a separator", parts: []string{"a-1"}, want: "A1"},
		{name: "Negative decimal", parts: []string{"-0.5"}, want: "Minus0Dot5"},
		{name: "Leading plus", parts: []string{"+1"}, want: "Plus1"},
		{name: "Plus inside a name", parts: []string{"C++"}, want: "CPlusPlus"},
		{name: "At sign", parts: []string{"@type"}, want: "AtType"},
		{name: "At sign inside a name", parts: []string{"user@host"}, want: "UserAtHost"},
		{name: "Dollar with letters is a separator", parts: []string{"$ref"}, want: "Ref"},
		{name: "Lone dollar", parts: []string{"$"}, want: "Dollar"},
		{name: "Lone minus", parts: []string{"-"}, want: "Minus"},
		{name: "Lone underscore", parts: []string{"_"}, want: "Underscore"},
		{name: "Symbols only", parts: []string{">="}, want: "GreaterThanEqual"},
		{name: "Not equal", parts: []string{"!="}, want: "NotEqual"},
		{name: "Symbols after a separator", parts: []string{"<-"}, want: "LessThanMinus"},
		{name: "Space only", parts: []string{" "}, want: "Space"},
		{name: "Every symbol word", parts: []string{"\"#%&'()*,/:;?[\\]^`{|}~"}, want: "QuoteHashPercentAndApostropheLeftParenRightParenAsteriskCommaSlashColonSemicolonQuestionLeftBracketBackslashRightBracketCaretBacktickLeftBraceOrRightBraceTilde"},
		{name: "Latin diacritics fold", parts: []string{"naïve café"}, want: "NaiveCafe"},
		{name: "Sharp s", parts: []string{"Straße"}, want: "Strasse"},
		{name: "Upper-case diacritics fold", parts: []string{"ÉCOLE"}, want: "Ecole"},
		{name: "Ligature", parts: []string{"Ærø"}, want: "Aero"},
		{name: "Other scripts are dropped", parts: []string{"日本語Name"}, want: "Name"},
		{name: "Other scripts only give hex of the first rune", parts: []string{"名前"}, want: "X540D"},
		{name: "Emoji gives hex", parts: []string{"😀"}, want: "X1F600"},
		{name: "Control character gives hex", parts: []string{"\t"}, want: "X9"},
		{name: "Empty input", parts: []string{""}, want: "Empty"},
		{name: "No parts", want: "Empty"},
		{name: "Parts are joined", parts: []string{"Order", "client_address"}, want: "OrderClientAddress"},
		{name: "Empty part", parts: []string{"Status", ""}, want: "StatusEmpty"},
		{name: "Digit part after a word gets no N", parts: []string{"Response", "200"}, want: "Response200"},
		{name: "Digit first part gets N", parts: []string{"2", "Fast"}, want: "N2Fast"},
		{name: "Keyword needs no suffix when exported", parts: []string{"type"}, want: "Type"},
		{name: "Media type", parts: []string{"application/json"}, want: "ApplicationJSON"},
		{name: "Extension name", parts: []string{"x-go-name"}, want: "XGoName"},
	}
	n := New(nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, n.Exported(tt.parts...))
		})
	}
}

func TestExportedWithExtraInitialisms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		extra []string
		in    string
		want  string
	}{
		{name: "Extra initialism", extra: []string{"PSP"}, in: "pspId", want: "PSPID"},
		{name: "Extra initialism given in lower case is written upper case", extra: []string{"ssn"}, in: "SSN", want: "SSN"},
		{name: "Extra initialism plural", extra: []string{"PSP"}, in: "PSPs", want: "PSPs"},
		{name: "Empty extra is ignored", extra: []string{""}, in: "s", want: "S"},
		{name: "Defaults stay with extras", extra: []string{"PSP"}, in: "url", want: "URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, New(tt.extra).Exported(tt.in))
		})
	}
}

func TestUnexported(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		parts []string
		want  string
	}{
		{name: "Camel case", parts: []string{"PetName"}, want: "petName"},
		{name: "Snake case", parts: []string{"pet_name"}, want: "petName"},
		{name: "Leading initialism is lower case", parts: []string{"HTTPServer"}, want: "httpServer"},
		{name: "Initialism alone", parts: []string{"ID"}, want: "id"},
		{name: "Plural initialism alone", parts: []string{"ids"}, want: "ids"},
		{name: "Later initialism keeps its case", parts: []string{"user_id"}, want: "userID"},
		{name: "Leading digit gets n", parts: []string{"1st"}, want: "n1st"},
		{name: "Keyword gets Val", parts: []string{"type"}, want: "typeVal"},
		{name: "Keyword in title case gets Val", parts: []string{"Default"}, want: "defaultVal"},
		{name: "Predeclared type gets Val", parts: []string{"string"}, want: "stringVal"},
		{name: "Predeclared function gets Val", parts: []string{"len"}, want: "lenVal"},
		{name: "Predeclared constant gets Val", parts: []string{"nil"}, want: "nilVal"},
		{name: "Keyword inside a longer name is fine", parts: []string{"type_name"}, want: "typeName"},
		{name: "Symbols only", parts: []string{"$"}, want: "dollar"},
		{name: "Empty input", parts: []string{""}, want: "empty"},
		{name: "No parts", want: "empty"},
		{name: "Parts are joined", parts: []string{"page", "size"}, want: "pageSize"},
	}
	n := New(nil)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, n.Unexported(tt.parts...))
		})
	}
}

func TestSuffixes(t *testing.T) {
	t.Parallel()

	n := New(nil)
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "Inline property", got: n.InlineProperty("Order", "client_address"), want: "OrderClientAddress"},
		{name: "Inline property keeps the parent as is", got: n.InlineProperty("XMLHttpRequest", "id"), want: "XMLHttpRequestID"},
		{name: "Inline property with a leading digit gets no N", got: n.InlineProperty("Pet", "1st"), want: "Pet1st"},
		{name: "Array item", got: n.ArrayItem("Order"), want: "OrderItem"},
		{name: "Map value", got: n.MapValue("Labels"), want: "LabelsValue"},
		{name: "Union variant by title", got: n.UnionVariant("PaymentMethod", "card", "bank", 0), want: "PaymentMethodCard"},
		{name: "Union variant by discriminator value", got: n.UnionVariant("PaymentMethod", "", "bank_account", 1), want: "PaymentMethodBankAccount"},
		{name: "Union variant by position from 1", got: n.UnionVariant("PaymentMethod", "", "", 1), want: "PaymentMethodOption2"},
		{name: "Only request body", got: n.RequestBody("CreatePet", "application/json", false), want: "CreatePetRequestBody"},
		{name: "JSON request body among several", got: n.RequestBody("CreatePet", "application/json; charset=utf-8", true), want: "CreatePetJSONRequestBody"},
		{name: "Structured suffix media type", got: n.RequestBody("CreatePet", "application/vnd.api+json", true), want: "CreatePetVndAPIJSONRequestBody"},
		{name: "Form media type", got: n.RequestBody("CreatePet", "application/x-www-form-urlencoded", true), want: "CreatePetFormRequestBody"},
		{name: "Multipart media type", got: n.RequestBody("CreatePet", "multipart/form-data", true), want: "CreatePetMultipartRequestBody"},
		{name: "Plain text media type", got: n.RequestBody("CreatePet", "text/plain", true), want: "CreatePetTextRequestBody"},
		{name: "Media type with an x- subtype", got: n.RequestBody("CreatePet", "application/x-ndjson", true), want: "CreatePetNdjsonRequestBody"},
		{name: "Media type with a wildcard subtype", got: n.RequestBody("CreatePet", "image/*", true), want: "CreatePetImageRequestBody"},
		{name: "Media type that is all wildcards", got: n.RequestBody("CreatePet", "*/*", true), want: "CreatePetAnyRequestBody"},
		{name: "Response by status", got: n.Response("GetPet", "200", "application/json", false), want: "GetPetResponse200"},
		{name: "Response by status range", got: n.Response("GetPet", "4xx", "application/json", false), want: "GetPetResponse4XX"},
		{name: "Default response", got: n.Response("GetPet", "Default", "application/json", false), want: "GetPetResponseDefault"},
		{name: "Response headers", got: n.ResponseHeaders("GetPet", "200"), want: "GetPetResponse200Headers"},
		{name: "Response item", got: n.ResponseItem("GetEvents"), want: "GetEventsResponseItem"},
		{name: "Only response constructor", got: n.ResponseConstructor("GetPet", "200", false), want: "NewGetPetResponseData"},
		{name: "Response constructor among several", got: n.ResponseConstructor("GetPet", "default", true), want: "NewGetPetResponseDataDefault"},
		{name: "JSON response among several", got: n.Response("GetPet", "200", "application/json", true), want: "GetPetJSONResponse200"},
		{name: "Default XML response among several", got: n.Response("GetPet", "default", "application/xml", true), want: "GetPetXMLResponseDefault"},
		{name: "Path params", got: n.Params("GetPet", "path"), want: "GetPetPathParams"},
		{name: "Query params", got: n.Params("GetPet", "query"), want: "GetPetQuery"},
		{name: "Header params", got: n.Params("GetPet", "header"), want: "GetPetHeaders"},
		{name: "Cookie params", got: n.Params("GetPet", "cookie"), want: "GetPetCookies"},
		{name: "Params in an unknown location", got: n.Params("GetPet", "querystring"), want: "GetPetQuerystringParams"},
		{name: "Service request options", got: n.ServiceRequestOptions("GetPet"), want: "GetPetServiceRequestOptions"},
		{name: "Response data", got: n.ResponseData("GetPet"), want: "GetPetResponseData"},
		{name: "Client request options", got: n.ClientRequestOptions("GetPet"), want: "GetPetRequestOptions"},
		{name: "Client response", got: n.ClientResponse("GetPet"), want: "GetPetResponse"},
		{name: "Client option", got: n.ClientOption("PetClient"), want: "PetClientOption"},
		{name: "Interface", got: n.Interface("PetClient"), want: "PetClientInterface"},
		{name: "Tool input", got: n.ToolInput("GetPet"), want: "GetPetToolInput"},
		{name: "Enum constant", got: n.EnumConst("Status", "in_progress"), want: "StatusInProgress"},
		{name: "Enum constant for a number", got: n.EnumConst("Level", "-1"), want: "LevelMinus1"},
		{name: "Enum constant for an empty string", got: n.EnumConst("Status", ""), want: "StatusEmpty"},
		{name: "Enum values func", got: n.EnumValues("Status"), want: "StatusValues"},
		{name: "Snake case of a Go name", got: n.Snake("GetPetByID"), want: "get_pet_by_id"},
		{name: "Snake case keeps an initialism with digits whole", got: n.Snake("HTTP2Stats"), want: "http2_stats"},
		{name: "Snake case of a plural initialism", got: n.Snake("ListUserIDs"), want: "list_user_ids"},
		{name: "Snake case of one word", got: n.Snake("Ping"), want: "ping"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.got)
		})
	}
}
