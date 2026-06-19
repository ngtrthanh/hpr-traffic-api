package main

// icaoToCountry maps ICAO airport prefix (1-2 chars) to ISO 3166-1 alpha-2 country code.
// Longest prefix match wins (e.g., "EG" before "E").
var icaoToCountry = map[string]string{
	// A — Western South Pacific
	"AG": "SB", // Solomon Islands
	"AN": "NR", // Nauru
	"AY": "PG", // Papua New Guinea
	// B — Iceland/Greenland/Kosovo
	"BG": "GL", // Greenland
	"BI": "IS", // Iceland
	"BK": "XK", // Kosovo
	// C — Canada
	"C":  "CA",
	// D — West Africa
	"DA": "DZ", // Algeria
	"DB": "BJ", // Benin
	"DF": "BF", // Burkina Faso
	"DG": "GH", // Ghana
	"DI": "CI", // Côte d'Ivoire
	"DN": "NG", // Nigeria
	"DR": "NE", // Niger
	"DT": "TN", // Tunisia
	"DX": "TG", // Togo
	// E — Northern Europe
	"EB": "BE", // Belgium
	"ED": "DE", // Germany (civil)
	"EE": "EE", // Estonia
	"EF": "FI", // Finland
	"EG": "GB", // United Kingdom
	"EH": "NL", // Netherlands
	"EI": "IE", // Ireland
	"EK": "DK", // Denmark
	"EL": "LU", // Luxembourg
	"EN": "NO", // Norway
	"EP": "PL", // Poland
	"ES": "SE", // Sweden
	"ET": "DE", // Germany (military)
	"EV": "LV", // Latvia
	"EY": "LT", // Lithuania
	// F — Central/Southern Africa
	"FA": "ZA", // South Africa
	"FB": "BW", // Botswana
	"FC": "CG", // Congo
	"FD": "SZ", // Eswatini
	"FE": "CF", // Central African Republic
	"FG": "GQ", // Equatorial Guinea
	"FH": "SH", // Saint Helena
	"FI": "MU", // Mauritius
	"FJ": "IO", // British Indian Ocean Territory
	"FK": "CM", // Cameroon
	"FL": "ZM", // Zambia
	"FM": "MG", // Madagascar (also Comoros, Réunion, Mayotte)
	"FN": "AO", // Angola
	"FO": "GA", // Gabon
	"FP": "ST", // São Tomé and Príncipe
	"FQ": "MZ", // Mozambique
	"FS": "SC", // Seychelles
	"FT": "TD", // Chad
	"FV": "ZW", // Zimbabwe
	"FW": "MW", // Malawi
	"FX": "LS", // Lesotho
	"FY": "NA", // Namibia
	"FZ": "CD", // DR Congo
	// G — West Africa / Maghreb
	"GA": "ML", // Mali
	"GB": "GM", // Gambia
	"GC": "ES", // Canary Islands (Spain)
	"GE": "ES", // Spain (Ceuta/Melilla)
	"GF": "SL", // Sierra Leone
	"GG": "GW", // Guinea-Bissau
	"GL": "LR", // Liberia
	"GM": "MA", // Morocco
	"GO": "SN", // Senegal
	"GQ": "MR", // Mauritania
	"GS": "EH", // Western Sahara
	"GU": "GN", // Guinea
	"GV": "CV", // Cape Verde
	// H — East Africa / Horn
	"HA": "ET", // Ethiopia
	"HB": "BI", // Burundi
	"HC": "SO", // Somalia
	"HD": "DJ", // Djibouti
	"HE": "EG", // Egypt
	"HH": "ER", // Eritrea
	"HK": "KE", // Kenya
	"HL": "LY", // Libya
	"HR": "RW", // Rwanda
	"HS": "SD", // Sudan
	"HT": "TZ", // Tanzania
	"HU": "UG", // Uganda
	// K — Contiguous United States
	"K":  "US",
	// L — Southern Europe / Mediterranean
	"LA": "AL", // Albania
	"LB": "BG", // Bulgaria
	"LC": "CY", // Cyprus
	"LD": "HR", // Croatia
	"LE": "ES", // Spain
	"LF": "FR", // France
	"LG": "GR", // Greece
	"LH": "HU", // Hungary
	"LI": "IT", // Italy
	"LJ": "SI", // Slovenia
	"LK": "CZ", // Czech Republic
	"LL": "IL", // Israel
	"LM": "MT", // Malta
	"LN": "MC", // Monaco
	"LO": "AT", // Austria
	"LP": "PT", // Portugal
	"LQ": "BA", // Bosnia and Herzegovina
	"LR": "RO", // Romania
	"LS": "CH", // Switzerland
	"LT": "TR", // Turkey
	"LU": "MD", // Moldova
	"LW": "MK", // North Macedonia
	"LX": "GI", // Gibraltar
	"LY": "RS", // Serbia
	"LZ": "SK", // Slovakia
	// M — Central America / Caribbean
	"MB": "AN", // Turks and Caicos
	"MD": "DO", // Dominican Republic
	"MG": "GT", // Guatemala
	"MH": "HN", // Honduras
	"MK": "JM", // Jamaica
	"MM": "MX", // Mexico
	"MN": "NI", // Nicaragua
	"MP": "PA", // Panama
	"MR": "CR", // Costa Rica
	"MS": "SV", // El Salvador
	"MT": "HT", // Haiti
	"MU": "CU", // Cuba
	"MW": "KY", // Cayman Islands
	"MY": "BS", // Bahamas
	"MZ": "BZ", // Belize
	// N — South Pacific
	"NC": "CK", // Cook Islands
	"NF": "FJ", // Fiji
	"NI": "NU", // Niue
	"NL": "WF", // Wallis and Futuna
	"NS": "WS", // Samoa
	"NT": "PF", // French Polynesia
	"NV": "VU", // Vanuatu
	"NW": "NC", // New Caledonia
	"NZ": "NZ", // New Zealand
	// O — Middle East / South Asia
	"OA": "AF", // Afghanistan
	"OB": "BH", // Bahrain
	"OE": "SA", // Saudi Arabia
	"OI": "IR", // Iran
	"OJ": "JO", // Jordan
	"OK": "KW", // Kuwait
	"OL": "LB", // Lebanon
	"OM": "AE", // United Arab Emirates
	"OO": "OM", // Oman
	"OP": "PK", // Pakistan
	"OR": "IQ", // Iraq
	"OS": "SY", // Syria
	"OT": "QA", // Qatar
	"OY": "YE", // Yemen
	// P — North Pacific
	"PA": "US", // Alaska
	"PG": "US", // Guam / N. Mariana Islands
	"PH": "US", // Hawaii
	"PJ": "US", // Johnston Atoll
	"PK": "MH", // Marshall Islands
	"PL": "KI", // Kiribati (Line Islands)
	"PM": "US", // Midway
	"PT": "FM", // Micronesia
	"PW": "PW", // Palau
	// R — East Asia
	"RC": "TW", // Taiwan
	"RJ": "JP", // Japan (civil)
	"RK": "KR", // South Korea
	"RO": "JP", // Japan (military)
	"RP": "PH", // Philippines
	// S — South America
	"SA": "AR", // Argentina
	"SB": "BR", // Brazil
	"SC": "CL", // Chile
	"SD": "BR", // Brazil
	"SE": "EC", // Ecuador
	"SG": "PY", // Paraguay
	"SK": "CO", // Colombia
	"SL": "BO", // Bolivia
	"SM": "SR", // Suriname
	"SN": "BR", // Brazil
	"SO": "FR", // French Guiana
	"SP": "PE", // Peru
	"SS": "BR", // Brazil
	"SU": "UY", // Uruguay
	"SV": "VE", // Venezuela
	"SW": "BR", // Brazil
	"SY": "GY", // Guyana
	// T — Caribbean
	"TA": "AG", // Antigua and Barbuda
	"TB": "BB", // Barbados
	"TD": "DM", // Dominica
	"TF": "FR", // Guadeloupe / Martinique / Saint Barthélemy / Saint Martin
	"TG": "GD", // Grenada
	"TI": "VI", // US Virgin Islands
	"TJ": "PR", // Puerto Rico
	"TK": "KN", // Saint Kitts and Nevis
	"TL": "LC", // Saint Lucia
	"TN": "AW", // Caribbean Netherlands / Aruba / Curaçao
	"TQ": "AI", // Anguilla
	"TR": "MS", // Montserrat
	"TT": "TT", // Trinidad and Tobago
	"TU": "GB", // British Virgin Islands
	"TV": "VC", // Saint Vincent
	"TX": "BM", // Bermuda
	// U — Russia and former USSR
	"UA": "KZ", // Kazakhstan
	"UB": "AZ", // Azerbaijan
	"UC": "KG", // Kyrgyzstan
	"UD": "AM", // Armenia
	"UE": "RU", // Russia
	"UG": "GE", // Georgia
	"UH": "RU", // Russia
	"UI": "RU", // Russia
	"UK": "UA", // Ukraine
	"UL": "RU", // Russia
	"UM": "BY", // Belarus
	"UN": "RU", // Russia
	"UO": "RU", // Russia
	"UR": "RU", // Russia
	"US": "RU", // Russia
	"UT": "TJ", // Tajikistan / Turkmenistan / Uzbekistan
	"UU": "RU", // Russia
	"UW": "RU", // Russia
	// V — South/Southeast Asia
	"VA": "IN", // India (West)
	"VC": "LK", // Sri Lanka
	"VD": "KH", // Cambodia
	"VE": "IN", // India (East)
	"VG": "BD", // Bangladesh
	"VH": "HK", // Hong Kong
	"VI": "IN", // India (North)
	"VL": "LA", // Laos
	"VM": "MO", // Macau
	"VN": "NP", // Nepal
	"VO": "IN", // India (South)
	"VQ": "BT", // Bhutan
	"VR": "MV", // Maldives
	"VT": "TH", // Thailand
	"VV": "VN", // Vietnam
	"VY": "MM", // Myanmar
	// W — Maritime Southeast Asia
	"WA": "ID", // Indonesia
	"WB": "MY", // Malaysia (East) / Brunei
	"WI": "ID", // Indonesia
	"WM": "MY", // Malaysia (West)
	"WP": "TL", // Timor-Leste
	"WR": "ID", // Indonesia
	"WS": "SG", // Singapore
	// Y — Australia
	"Y":  "AU",
	// Z — China / North Korea / Mongolia
	"ZB": "CN", // China
	"ZG": "CN", // China
	"ZH": "CN", // China
	"ZJ": "CN", // China
	"ZK": "KP", // North Korea
	"ZL": "CN", // China
	"ZM": "MN", // Mongolia
	"ZP": "CN", // China
	"ZS": "CN", // China
	"ZU": "CN", // China
	"ZW": "CN", // China
	"ZY": "CN", // China
}

// ituToCountry maps ITU 3-letter administration codes to ISO 3166-1 alpha-2.
var ituToCountry = map[string]string{
	"AFG": "AF", "ALB": "AL", "ALG": "DZ", "AND": "AD", "AGL": "AO",
	"ARG": "AR", "ARM": "AM", "AUS": "AU", "AUT": "AT", "AZE": "AZ",
	"BAH": "BS", "BHR": "BH", "BGD": "BD", "BRB": "BB", "BLR": "BY",
	"BEL": "BE", "BLZ": "BZ", "BEN": "BJ", "BTN": "BT", "BOL": "BO",
	"BIH": "BA", "BOT": "BW", "B":   "BR", "BRU": "BN", "BUL": "BG",
	"BFA": "BF", "BDI": "BI", "CPV": "CV", "CBG": "KH", "CME": "CM",
	"CAN": "CA", "CAF": "CF", "TCD": "TD", "CHL": "CL", "CHN": "CN",
	"CLM": "CO", "COM": "KM", "COG": "CG", "COD": "CD", "CRI": "CR",
	"CIV": "CI", "HRV": "HR", "CUB": "CU", "CYP": "CY", "CZE": "CZ",
	"D":   "DE", "DNK": "DK", "DJI": "DJ", "DMA": "DM", "DOM": "DO",
	"ECU": "EC", "EGY": "EG", "SLV": "SV", "GNE": "GQ", "ERI": "ER",
	"EST": "EE", "ETH": "ET", "FJI": "FJ", "FIN": "FI", "F":   "FR",
	"GAB": "GA", "GMB": "GM", "GEO": "GE", "GHA": "GH", "GRC": "GR",
	"GRD": "GD", "GTM": "GT", "GUI": "GN", "GUY": "GY", "HTI": "HT",
	"HND": "HN", "HNG": "HU", "ISL": "IS", "IND": "IN", "INS": "ID",
	"IRN": "IR", "IRQ": "IQ", "IRL": "IE", "ISR": "IL", "I":   "IT",
	"JAM": "JM", "J":   "JP", "JOR": "JO", "KAZ": "KZ", "KEN": "KE",
	"KIR": "KI", "KOR": "KR", "KWT": "KW", "KGZ": "KG", "LAO": "LA",
	"LVA": "LV", "LBN": "LB", "LSO": "LS", "LBR": "LR", "LBY": "LY",
	"LIE": "LI", "LTU": "LT", "LUX": "LU", "MDG": "MG", "MWI": "MW",
	"MLA": "MY", "MDV": "MV", "MLI": "ML", "MLT": "MT", "MHL": "MH",
	"MTN": "MR", "MRC": "MU", "MEX": "MX", "FSM": "FM", "MDA": "MD",
	"MCO": "MC", "MNG": "MN", "MNE": "ME", "MOR": "MA", "MOZ": "MZ",
	"MYA": "MM", "NMB": "NA", "NRU": "NR", "NPL": "NP", "HOL": "NL",
	"NZL": "NZ", "NCG": "NI", "NGR": "NE", "NIG": "NG", "NOR": "NO",
	"OMA": "OM", "PAK": "PK", "PLW": "PW", "PNR": "PA", "PNG": "PG",
	"PRG": "PY", "PRU": "PE", "PHL": "PH", "POL": "PL", "POR": "PT",
	"QAT": "QA", "ROU": "RO", "RUS": "RU", "RRW": "RW", "KNA": "KN",
	"LCA": "LC", "VCT": "VC", "SMO": "WS", "SMR": "SM", "STP": "ST",
	"ARS": "SA", "SEN": "SN", "SRB": "RS", "SEY": "SC", "SRL": "SL",
	"SNG": "SG", "SVK": "SK", "SVN": "SI", "SLM": "SB", "SOM": "SO",
	"AFS": "ZA", "SSD": "SS", "E":   "ES", "CLN": "LK", "SDN": "SD",
	"SUR": "SR", "SWZ": "SZ", "S":   "SE", "SUI": "CH", "SYR": "SY",
	"TJK": "TJ", "TZA": "TZ", "THA": "TH", "TLS": "TL", "TGO": "TG",
	"TON": "TO", "TRD": "TT", "TUN": "TN", "TUR": "TR", "TKM": "TM",
	"TUV": "TV", "UGA": "UG", "UKR": "UA", "UAE": "AE", "G":   "GB",
	"USA": "US", "URG": "UY", "UZB": "UZ", "VUT": "VU", "VEN": "VE",
	"VTN": "VN", "YEM": "YE", "ZMB": "ZM", "ZWE": "ZW",
	// Common flag-of-convenience registries (additional codes)
	"BHS": "BS", // Bahamas (alt)
	"HKG": "HK", // Hong Kong
	"SGP": "SG", // Singapore
	"BMU": "BM", // Bermuda
	"ATG": "AG", // Antigua and Barbuda
	"GIB": "GI", // Gibraltar
	"CYM": "KY", // Cayman Islands
	"ISM": "GB", // Isle of Man
	"DIS": "DK", // Denmark (DIS register)
	"NIS": "NO", // Norway (NIS register)
	"FIS": "FI", // Finland (ship register)
	"MAD": "MH", // Marshall Islands (alt)
}

// icaoCountryCode resolves an ICAO airport code to ISO-2 country code.
func icaoCountryCode(icao string) string {
	if len(icao) < 2 {
		return ""
	}
	// Try 2-char prefix first (most specific)
	if cc, ok := icaoToCountry[icao[:2]]; ok {
		return cc
	}
	// Try 1-char prefix
	if cc, ok := icaoToCountry[icao[:1]]; ok {
		return cc
	}
	return ""
}

// ituCountryCode resolves ITU 3-letter administration code to ISO-2.
func ituCountryCode(itu string) string {
	if cc, ok := ituToCountry[itu]; ok {
		return cc
	}
	return ""
}
