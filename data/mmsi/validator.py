"""IMO and MMSI Validator.

This module provides comprehensive validation for maritime vessel identifiers:
- MMSI (Maritime Mobile Service Identity) - 9-digit radio communication identifier
- IMO (International Maritime Organization) Number - 7-digit permanent vessel identifier

Implements full validation rules including:
- Format validation
- Check digit verification (IMO only)
- MID (Maritime Identification Digits) country code validation
- Type detection and classification
- Warnings for suspicious patterns and edge cases

Author: Scitus Solutions Ltd.
License: MIT
Version: 1.0.0
"""

from typing import Dict, List, Optional, Any, Literal
from dataclasses import dataclass, field


__version__ = "0.1.1"
__author__ = "Scitus Solutions Ltd."
__license__ = "MIT"


@dataclass
class ValidationResult:
    """Result of vessel identifier validation."""

    valid: bool
    normalized: str = ""
    errors: List[str] = field(default_factory=list)
    warnings: List[str] = field(default_factory=list)
    info: Dict[str, Any] = field(default_factory=dict)


class VesselIdentifierValidator:
    """Validates MMSI and IMO numbers according to international maritime standards."""

    # MID (Maritime Identification Digits) to Country mapping
    # Full list available at https://www.itu.int/en/ITU-R/terrestrial/fmd/Pages/mid.aspx
    MID_COUNTRIES = {
        # North America
        "303": "United States of America",
        "304": "United States of America",
        "305": "United States of America",
        "306": "Netherlands Antilles",
        "307": "Netherlands Antilles",
        "308": "Bahamas",
        "309": "Bahamas",
        "310": "Bermuda",
        "311": "Bahamas",
        "312": "Belize",
        "314": "Barbados",
        "316": "Canada",
        "319": "Cayman Islands",
        "321": "Costa Rica",
        "323": "Cuba",
        "325": "Dominica",
        "327": "Dominican Republic",
        "329": "Guadeloupe",
        "330": "Grenada",
        "331": "Greenland",
        "332": "Guatemala",
        "334": "Honduras",
        "336": "Haiti",
        "338": "United States of America",
        "339": "Jamaica",
        "341": "Saint Kitts and Nevis",
        "343": "Saint Lucia",
        "345": "Mexico",
        "347": "Martinique",
        "348": "Montserrat",
        "350": "Nicaragua",
        "351": "Panama",
        "352": "Panama",
        "353": "Panama",
        "354": "Panama",
        "355": "Panama",
        "356": "Panama",
        "357": "Panama",
        "358": "Puerto Rico",
        "359": "El Salvador",
        "361": "Saint Pierre and Miquelon",
        "362": "Trinidad and Tobago",
        "364": "Turks and Caicos Islands",
        "366": "United States of America",
        "367": "United States of America",
        "368": "United States of America",
        "369": "United States of America",
        "370": "Panama",
        "371": "Panama",
        "372": "Panama",
        "373": "Panama",
        "374": "Panama",
        "375": "Saint Vincent and the Grenadines",
        "376": "Saint Vincent and the Grenadines",
        "377": "Saint Vincent and the Grenadines",
        "378": "British Virgin Islands",
        # Europe
        "201": "Albania",
        "202": "Andorra",
        "203": "Austria",
        "204": "Azores",
        "205": "Belgium",
        "206": "Belarus",
        "207": "Bulgaria",
        "208": "Vatican City State",
        "209": "Cyprus",
        "210": "Cyprus",
        "211": "Germany",
        "212": "Cyprus",
        "213": "Georgia",
        "214": "Moldova",
        "215": "Malta",
        "216": "Armenia",
        "218": "Germany",
        "219": "Denmark",
        "220": "Denmark",
        "224": "Spain",
        "225": "Spain",
        "226": "France",
        "227": "France",
        "228": "France",
        "229": "Malta",
        "230": "Finland",
        "231": "Faroe Islands",
        "232": "United Kingdom",
        "233": "United Kingdom",
        "234": "United Kingdom",
        "235": "United Kingdom",
        "236": "Gibraltar",
        "237": "Greece",
        "238": "Croatia",
        "239": "Greece",
        "240": "Greece",
        "241": "Greece",
        "242": "Morocco",
        "243": "Hungary",
        "244": "Netherlands",
        "245": "Netherlands",
        "246": "Netherlands",
        "247": "Italy",
        "248": "Malta",
        "249": "Malta",
        "250": "Ireland",
        "251": "Iceland",
        "252": "Liechtenstein",
        "253": "Luxembourg",
        "254": "Monaco",
        "255": "Madeira",
        "256": "Malta",
        "257": "Norway",
        "258": "Norway",
        "259": "Norway",
        "261": "Poland",
        "262": "Montenegro",
        "263": "Portugal",
        "264": "Romania",
        "265": "Sweden",
        "266": "Sweden",
        "267": "Slovak Republic",
        "268": "San Marino",
        "269": "Switzerland",
        "270": "Czech Republic",
        "271": "Turkey",
        "272": "Ukraine",
        "273": "Russian Federation",
        "274": "Macedonia",
        "275": "Latvia",
        "276": "Estonia",
        "277": "Lithuania",
        "278": "Slovenia",
        "279": "Serbia",
        # Asia
        "401": "Afghanistan",
        "403": "Saudi Arabia",
        "405": "Bangladesh",
        "408": "Bahrain",
        "410": "Bhutan",
        "412": "China",
        "413": "China",
        "414": "China",
        "416": "Taiwan",
        "417": "Sri Lanka",
        "419": "India",
        "422": "Iran",
        "423": "Azerbaijan",
        "425": "Iraq",
        "428": "Israel",
        "431": "Japan",
        "432": "Japan",
        "434": "Turkmenistan",
        "436": "Kazakhstan",
        "437": "Uzbekistan",
        "438": "Jordan",
        "440": "Korea",
        "441": "Korea",
        "443": "Palestine",
        "445": "Democratic People's Republic of Korea",
        "447": "Kuwait",
        "450": "Lebanon",
        "451": "Kyrgyz Republic",
        "453": "Macao",
        "455": "Maldives",
        "457": "Mongolia",
        "459": "Nepal",
        "461": "Oman",
        "463": "Pakistan",
        "466": "Qatar",
        "468": "Syrian Arab Republic",
        "470": "United Arab Emirates",
        "471": "United Arab Emirates",
        "472": "Tajikistan",
        "473": "Yemen",
        "475": "Yemen",
        "477": "Hong Kong",
        "478": "Bosnia and Herzegovina",
        # Oceania
        "503": "Australia",
        "506": "Myanmar",
        "508": "Brunei Darussalam",
        "510": "Micronesia",
        "511": "Palau",
        "512": "New Zealand",
        "514": "Cambodia",
        "515": "Cambodia",
        "516": "Christmas Island",
        "518": "Cook Islands",
        "520": "Fiji",
        "523": "Cocos (Keeling) Islands",
        "525": "Indonesia",
        "529": "Kiribati",
        "531": "Lao People's Democratic Republic",
        "533": "Malaysia",
        "536": "Northern Mariana Islands",
        "538": "Marshall Islands",
        "540": "New Caledonia",
        "542": "Niue",
        "544": "Nauru",
        "546": "French Polynesia",
        "548": "Philippines",
        "553": "Papua New Guinea",
        "555": "Pitcairn Island",
        "557": "Solomon Islands",
        "559": "American Samoa",
        "561": "Samoa",
        "563": "Singapore",
        "564": "Singapore",
        "565": "Singapore",
        "566": "Singapore",
        "567": "Thailand",
        "570": "Tonga",
        "572": "Tuvalu",
        "574": "Viet Nam",
        "576": "Vanuatu",
        "577": "Vanuatu",
        "578": "Wallis and Futuna Islands",
        # Africa
        "601": "South Africa",
        "603": "Angola",
        "605": "Algeria",
        "607": "Saint Paul and Amsterdam Islands",
        "608": "Ascension Island",
        "609": "Burundi",
        "610": "Benin",
        "611": "Botswana",
        "612": "Central African Republic",
        "613": "Cameroon",
        "615": "Congo",
        "616": "Comoros",
        "617": "Cape Verde",
        "618": "Crozet Archipelago",
        "619": "Ivory Coast",
        "620": "Comoros",
        "621": "Djibouti",
        "622": "Egypt",
        "624": "Ethiopia",
        "625": "Eritrea",
        "626": "Gabonese Republic",
        "627": "Ghana",
        "629": "Gambia",
        "630": "Guinea-Bissau",
        "631": "Equatorial Guinea",
        "632": "Guinea",
        "633": "Burkina Faso",
        "634": "Kenya",
        "635": "Kerguelen Islands",
        "636": "Liberia",
        "637": "Liberia",
        "638": "South Sudan",
        "642": "Libya",
        "644": "Lesotho",
        "645": "Mauritius",
        "647": "Madagascar",
        "649": "Mali",
        "650": "Mozambique",
        "654": "Mauritania",
        "655": "Malawi",
        "656": "Niger",
        "657": "Nigeria",
        "659": "Namibia",
        "660": "Reunion",
        "661": "Rwanda",
        "662": "Sudan",
        "663": "Senegal",
        "664": "Seychelles",
        "665": "Saint Helena",
        "666": "Somalia",
        "667": "Sierra Leone",
        "668": "Sao Tome and Principe",
        "669": "Swaziland",
        "670": "Chad",
        "671": "Togolese Republic",
        "672": "Tunisia",
        "674": "Tanzania",
        "675": "Uganda",
        "676": "Democratic Republic of the Congo",
        "677": "Tanzania",
        "678": "Zambia",
        "679": "Zimbabwe",
        # South America
        "710": "Brazil",
        "720": "Bolivia",
        "725": "Chile",
        "730": "Colombia",
        "735": "Ecuador",
        "740": "Falkland Islands",
        "745": "Guiana",
        "750": "Guyana",
        "755": "Paraguay",
        "760": "Peru",
        "765": "Suriname",
        "770": "Uruguay",
        "775": "Venezuela",
    }

    def __init__(self):
        """Initialize the validator."""
        pass

    # ==================== MMSI Validation ====================

    def validate_mmsi(
        self,
        mmsi: str,
        mmsi_type: Literal[
            "ship", "coast", "group", "handheld", "sar", "aton", "auto"
        ] = "auto",
        expected_country: Optional[str] = None,
        strict_format: bool = False,
        check_database: bool = False,
    ) -> ValidationResult:
        """
        Validate MMSI (Maritime Mobile Service Identity) number.

        Args:
            mmsi: The MMSI number to validate (9 digits)
            mmsi_type: Expected MMSI type ('auto' to detect automatically)
            expected_country: Expected country name for MID validation
            strict_format: If True, reject any formatting issues
            check_database: If True, check for duplicates (not implemented here)

        Returns:
            ValidationResult with validation status, errors, warnings, and info
        """
        result = ValidationResult(valid=True)

        # Normalize input
        mmsi_normalized = str(mmsi).strip() if mmsi else ""
        result.normalized = mmsi_normalized

        # Check empty
        if not mmsi_normalized:
            result.valid = False
            result.errors.append("MMSI cannot be empty")
            return result

        # Check length
        if len(mmsi_normalized) != 9:
            result.valid = False
            result.errors.append(
                f"MMSI must be exactly 9 digits, got {len(mmsi_normalized)}"
            )
            return result

        # Check numeric
        if not mmsi_normalized.isdigit():
            result.valid = False
            result.errors.append("MMSI must contain only numeric digits")
            return result

        # Detect type
        detected_type = self._detect_mmsi_type(mmsi_normalized)
        result.info["type"] = detected_type

        # Validate based on type
        if mmsi_type != "auto" and mmsi_type != detected_type:
            result.warnings.append(
                f"Expected MMSI type '{mmsi_type}', but detected '{detected_type}'"
            )

        # Extract and validate MID
        mid = self._extract_mmsi_mid(mmsi_normalized, detected_type)
        if mid:
            result.info["mid"] = mid

            # Validate MID range (201-775)
            mid_num = int(mid)
            if mid_num < 201 or mid_num > 775:
                result.valid = False
                result.errors.append(
                    f"Invalid MID code: {mid}. Must be between 201-775"
                )
            else:
                # Get country for MID
                country = self.MID_COUNTRIES.get(mid, "Unknown")
                result.info["country"] = country
                result.info["suitable_for"] = self._get_mmsi_usage_contexts(
                    detected_type
                )

                # Check country mismatch
                if (
                    expected_country
                    and country != expected_country
                    and country != "Unknown"
                ):
                    result.warnings.append(
                        f"MID {mid} corresponds to {country}, expected {expected_country}"
                    )

        # Type-specific validations
        if detected_type == "ship_station":
            # Ship station cannot start with 0, 1, 8, or 9
            first_digit = mmsi_normalized[0]
            if first_digit in ("0", "1", "8", "9"):
                result.valid = False
                result.errors.append(
                    "Ship station MMSI cannot start with 0, 1, 8, or 9"
                )

            # First digit must be 2-7
            if first_digit not in ("2", "3", "4", "5", "6", "7"):
                result.valid = False
                result.errors.append("Ship station MMSI must start with digit 2-7")

            # Check international voyage compliance
            if not mmsi_normalized.endswith("000"):
                result.warnings.append(
                    "Ship MMSI does not end in 000. May not be valid for international voyages or Inmarsat"
                )

        elif detected_type == "coast_station":
            if not mmsi_normalized.startswith("00"):
                result.valid = False
                result.errors.append("Coast station MMSI must start with 00")

        elif detected_type == "group_station":
            if not mmsi_normalized.startswith("0") or mmsi_normalized.startswith("00"):
                result.valid = False
                result.errors.append("Group station MMSI must start with single 0")

        elif detected_type == "handheld":
            if not mmsi_normalized.startswith("8"):
                result.valid = False
                result.errors.append("Handheld MMSI must start with 8")

        elif detected_type == "sar_aircraft":
            if not mmsi_normalized.startswith("111"):
                result.valid = False
                result.errors.append("SAR aircraft MMSI must start with 111")

        elif detected_type == "aton":
            if not mmsi_normalized.startswith("99"):
                result.valid = False
                result.errors.append("AIS AtoN MMSI must start with 99")

        elif detected_type == "unknown":
            # Unknown type - likely invalid MMSI format
            result.valid = False
            first_digit = mmsi_normalized[0]
            if first_digit == "1":
                result.errors.append(
                    f"Invalid MMSI format: MMSI starting with '1' must be SAR aircraft (111XXXXXX). "
                    f"Ship stations must start with 2-7, not {first_digit}"
                )
            else:
                result.errors.append(
                    "Unknown MMSI type. Valid formats: Ship (2-7XX), Coast (00XX), Group (0XX), "
                    "Handheld (8XX), SAR (111XX), AtoN (99XX), SART (970XX), Craft (98XX)"
                )

        # Check for suspicious patterns
        self._check_mmsi_suspicious_patterns(mmsi_normalized, result)

        return result

    def _detect_mmsi_type(self, mmsi: str) -> str:
        """Detect MMSI type based on format."""
        if mmsi.startswith("00"):
            return "coast_station"
        elif mmsi.startswith("111"):
            return "sar_aircraft"
        elif mmsi.startswith("99"):
            return "aton"
        elif mmsi.startswith("98"):
            return "craft_associated_with_ship"
        elif mmsi.startswith("970"):
            return "ais_sart"
        elif mmsi.startswith("8"):
            return "handheld"
        elif mmsi.startswith("0"):
            return "group_station"
        elif mmsi[0] in ("2", "3", "4", "5", "6", "7"):
            return "ship_station"
        else:
            return "unknown"

    def _extract_mmsi_mid(self, mmsi: str, mmsi_type: str) -> Optional[str]:
        """Extract MID (Maritime Identification Digits) from MMSI."""
        if mmsi_type == "ship_station":
            return mmsi[0:3]
        elif mmsi_type == "coast_station":
            return mmsi[2:5]
        elif mmsi_type == "group_station":
            return mmsi[1:4]
        elif mmsi_type == "handheld":
            return mmsi[1:4]
        elif mmsi_type == "sar_aircraft":
            return mmsi[3:6]
        elif mmsi_type == "aton":
            return mmsi[2:5]
        elif mmsi_type == "craft_associated_with_ship":
            return mmsi[2:5]
        else:
            return None

    def _get_mmsi_usage_contexts(self, mmsi_type: str) -> List[str]:
        """Get suitable usage contexts for MMSI type."""
        contexts = {
            "ship_station": ["vhf_communication", "ais_transmission", "dsc_calling"],
            "coast_station": ["shore_based_communication", "traffic_management"],
            "group_station": ["fleet_operations", "group_calling"],
            "handheld": ["personal_vhf", "portable_equipment"],
            "sar_aircraft": ["search_and_rescue", "emergency_response"],
            "aton": ["navigation_aids", "buoy_identification"],
            "ais_sart": ["emergency_beacon", "distress_signal"],
            "craft_associated_with_ship": ["tender_operations", "pilot_boats"],
        }
        return contexts.get(mmsi_type, ["general_maritime_use"])

    def _check_mmsi_suspicious_patterns(
        self, mmsi: str, result: ValidationResult
    ) -> None:
        """Check for suspicious patterns in MMSI."""
        # All same digits
        if len(set(mmsi)) == 1:
            result.warnings.append(
                f"MMSI contains suspicious pattern (all same digits: {mmsi[0]})"
            )

        # Sequential ascending
        if mmsi in ("123456789", "012345678", "234567890"):
            result.warnings.append(
                "MMSI contains suspicious sequential ascending pattern"
            )

        # Sequential descending
        if mmsi in ("987654321", "876543210"):
            result.warnings.append(
                "MMSI contains suspicious sequential descending pattern"
            )

    # ==================== IMO Validation ====================

    def validate_imo(
        self,
        imo: str,
        number_type: Literal["ship", "company", "auto"] = "ship",
        strict_format: bool = False,
        allow_without_prefix: bool = True,
        check_database: bool = False,
    ) -> ValidationResult:
        """
        Validate IMO (International Maritime Organization) number.

        Args:
            imo: The IMO number to validate (e.g., "IMO 9074729" or "9074729")
            number_type: Type of IMO number ('ship', 'company', or 'auto')
            strict_format: If True, require "IMO" prefix
            allow_without_prefix: If True, accept numbers without "IMO" prefix
            check_database: If True, check for duplicates (not implemented here)

        Returns:
            ValidationResult with validation status, errors, warnings, and info
        """
        result = ValidationResult(valid=True)

        # Store original input for warning messages
        original_input = str(imo).strip() if imo else ""

        # Normalize input
        imo_normalized = original_input.upper()

        # Remove spaces and hyphens
        imo_normalized = imo_normalized.replace(" ", "").replace("-", "")

        # Check empty
        if not imo_normalized:
            result.valid = False
            result.errors.append("IMO number cannot be empty")
            return result

        # Check for IMO prefix
        has_prefix = imo_normalized.startswith("IMO")

        if has_prefix:
            # Remove prefix
            imo_digits = imo_normalized[3:]
        else:
            if strict_format and not allow_without_prefix:
                result.valid = False
                result.errors.append("IMO number must start with 'IMO' prefix")
                return result
            # Accept without prefix - we only care about validating the 7-digit number
            imo_digits = imo_normalized

        result.normalized = "IMO" + imo_digits

        # Check length
        if len(imo_digits) != 7:
            result.valid = False
            result.errors.append(
                f"IMO number must have exactly 7 digits after 'IMO' prefix, got {len(imo_digits)}"
            )
            return result

        # Check numeric
        if not imo_digits.isdigit():
            result.valid = False
            result.errors.append(
                "IMO number must contain only numeric digits after 'IMO' prefix"
            )
            return result

        # Check for leading zeros (suspicious)
        if imo_digits[0] == "0":
            result.errors.append("IMO number should not start with leading zeros")
            result.valid = False
            return result

        # Check valid range
        imo_num = int(imo_digits)
        if imo_num < 1000000:
            result.errors.append(
                f"IMO number {imo_num} is below valid range (suspicious)"
            )
            result.valid = False
            return result

        # Determine type if auto
        if number_type == "auto":
            # Default to ship unless we can determine otherwise
            number_type = "ship"
            result.info["type"] = "ship (assumed)"
        else:
            result.info["type"] = number_type

        # Validate check digit
        if number_type == "ship":
            calculated_check_digit = self._calculate_imo_ship_check_digit(
                imo_digits[:6]
            )
            actual_check_digit = int(imo_digits[6])

            result.info["check_digit"] = {
                "calculated": calculated_check_digit,
                "actual": actual_check_digit,
                "valid": calculated_check_digit == actual_check_digit,
            }

            if calculated_check_digit != actual_check_digit:
                result.valid = False
                result.errors.append(
                    f"Invalid IMO number: check digit validation failed. "
                    f"Expected {calculated_check_digit}, got {actual_check_digit}"
                )
            else:
                result.info["validation_success"] = (
                    f"IMO number {result.normalized} is valid (check digit verified)"
                )

        elif number_type == "company":
            calculated_check_digit = self._calculate_imo_company_check_digit(
                imo_digits[:6]
            )
            actual_check_digit = int(imo_digits[6])

            result.info["check_digit"] = {
                "calculated": calculated_check_digit,
                "actual": actual_check_digit,
                "valid": calculated_check_digit == actual_check_digit,
            }

            if calculated_check_digit != actual_check_digit:
                result.valid = False
                result.errors.append(
                    f"Invalid IMO company number: check digit validation failed. "
                    f"Expected {calculated_check_digit}, got {actual_check_digit}"
                )
            else:
                result.info["validation_success"] = (
                    f"IMO company number {result.normalized} is valid (check digit verified)"
                )

        # Historical warnings
        if imo_num < 6000000:
            result.warnings.append(
                f"IMO {imo_num} appears to be from an older vessel (pre-1990s)"
            )

        # Estimate era
        result.info["estimated_era"] = self._estimate_imo_era(imo_num)

        return result

    def _calculate_imo_ship_check_digit(self, first_six_digits: str) -> int:
        """
        Calculate IMO ship number check digit.

        Formula: (d1×7 + d2×6 + d3×5 + d4×4 + d5×3 + d6×2) mod 10

        Example: IMO 9074729
        Digits:    9    0    7    4    7    2
        Weights:   ×7   ×6   ×5   ×4   ×3   ×2
        Products:  63 + 0 + 35 + 16 + 21 + 4 = 139
        Check:     139 mod 10 = 9
        """
        if len(first_six_digits) != 6 or not first_six_digits.isdigit():
            return -1

        weights = [7, 6, 5, 4, 3, 2]
        total = sum(
            int(digit) * weight for digit, weight in zip(first_six_digits, weights)
        )
        return total % 10

    def _calculate_imo_company_check_digit(self, first_six_digits: str) -> int:
        """
        Calculate IMO company/owner number check digit.

        Formula: (11 - ((d1×8 + d2×6 + d3×4 + d4×2 + d5×9 + d6×7) mod 11)) mod 10

        Example: IMO 2041999
        Digits:    2    0    4    1    9    9
        Weights:   ×8   ×6   ×4   ×2   ×9   ×7
        Products:  16 + 0 + 16 + 2 + 81 + 63 = 178
        Step 1:    178 mod 11 = 2
        Step 2:    11 - 2 = 9
        Step 3:    9 mod 10 = 9
        """
        if len(first_six_digits) != 6 or not first_six_digits.isdigit():
            return -1

        weights = [8, 6, 4, 2, 9, 7]
        total = sum(
            int(digit) * weight for digit, weight in zip(first_six_digits, weights)
        )
        mod_11 = total % 11
        result = (11 - mod_11) % 10
        return result

    def _estimate_imo_era(self, imo_num: int) -> str:
        """Estimate vessel construction era based on IMO number range."""
        if imo_num < 5000000:
            return "Pre-1960s (historical)"
        elif imo_num < 6000000:
            return "1960s-1980s"
        elif imo_num < 7000000:
            return "1980s-1990s"
        elif imo_num < 8000000:
            return "1990s-2000s"
        elif imo_num < 9000000:
            return "2000s-2010s"
        elif imo_num < 9500000:
            return "2010s-2020s"
        else:
            return "2020s-present"

    # ==================== Batch Validation ====================

    def validate_mmsi_batch(self, mmsi_list: List[str], **kwargs) -> Dict[str, Any]:
        """
        Validate multiple MMSI numbers.

        Args:
            mmsi_list: List of MMSI numbers to validate
            **kwargs: Additional arguments passed to validate_mmsi()

        Returns:
            Dictionary with results for each MMSI and summary statistics
        """
        results = []
        total_valid = 0
        total_invalid = 0

        for mmsi in mmsi_list:
            result = self.validate_mmsi(mmsi, **kwargs)
            results.append({"mmsi": mmsi, "result": result})
            if result.valid:
                total_valid += 1
            else:
                total_invalid += 1

        return {
            "results": results,
            "summary": {
                "total": len(mmsi_list),
                "valid": total_valid,
                "invalid": total_invalid,
                "success_rate": (
                    f"{(total_valid / len(mmsi_list) * 100):.1f}%"
                    if mmsi_list
                    else "0%"
                ),
            },
        }

    def validate_imo_batch(self, imo_list: List[str], **kwargs) -> Dict[str, Any]:
        """
        Validate multiple IMO numbers.

        Args:
            imo_list: List of IMO numbers to validate
            **kwargs: Additional arguments passed to validate_imo()

        Returns:
            Dictionary with results for each IMO and summary statistics
        """
        results = []
        total_valid = 0
        total_invalid = 0

        for imo in imo_list:
            result = self.validate_imo(imo, **kwargs)
            results.append({"imo": imo, "result": result})
            if result.valid:
                total_valid += 1
            else:
                total_invalid += 1

        return {
            "results": results,
            "summary": {
                "total": len(imo_list),
                "valid": total_valid,
                "invalid": total_invalid,
                "success_rate": (
                    f"{(total_valid / len(imo_list) * 100):.1f}%" if imo_list else "0%"
                ),
            },
        }


# Create singleton instance
vessel_validator = VesselIdentifierValidator()


# Convenience functions for direct import
def validate_mmsi(mmsi: str, **kwargs) -> ValidationResult:
    """Validate MMSI number."""
    return vessel_validator.validate_mmsi(mmsi, **kwargs)


def validate_imo(imo: str, **kwargs) -> ValidationResult:
    """Validate IMO number."""
    return vessel_validator.validate_imo(imo, **kwargs)
