#!/usr/bin/env python3
"""Fix misclassified ports — override port_size and fill missing LOCODEs for world's top container ports."""
import csv

# World's top container ports (2023-2024 rankings) with correct classification
# Sources: Lloyd's List Top 100, UNCTAD, Drewry
FIXES = {
    # WPI_ID: { fields to override }
    # Format: (wpi_id, corrections_dict)
    # Top 10 worldwide
    "57680": {"port_size": "Major", "locode": "VNHPH", "max_vessel_size": "large vessels"},  # HAI PHONG - top 13
    "57857": {"port_size": "Major", "locode": "CNYTN", "max_vessel_size": "large vessels"},  # YANTIAN (Shenzhen) - top 5 
    "57777": {"port_size": "Major", "max_vessel_size": "large vessels"},  # SHEKOU (Shenzhen)
    "60140": {"port_size": "Major", "max_vessel_size": "large vessels"},  # QINGDAO - already Major but confirm
    "60390": {"port_size": "Major"},  # BUSAN/PUSAN - already Major
    # Severely underclassified
    "57543": {"port_size": "Major", "locode": "CNNBO", "max_vessel_size": "large vessels"},  # NINGBO - world #1 (need to find WPI)
    "48140": {"port_size": "Major", "locode": "SAJED", "max_vessel_size": "large vessels"},  # JIDDAH - top 30
    "48845": {"port_size": "Major", "max_vessel_size": "large vessels"},  # NHAVA SHEVA (JNPT) - top 25
    "45755": {"port_size": "Large", "max_vessel_size": "large vessels"},  # TANGER old port
    "45753": {"port_size": "Major", "locode": "MAPTM", "max_vessel_size": "large vessels"},  # TANGIER-MED - top 30
}

# Name-based fixes (for ports we can't easily find by WPI ID)
NAME_FIXES = {
    "NINGBO": {"port_size": "Major", "max_vessel_size": "large vessels"},
    "GUANGZHOU": {"port_size": "Major", "locode": "CNCAN", "max_vessel_size": "large vessels"},
    "XIAMEN": {"port_size": "Major", "locode": "CNXMN", "max_vessel_size": "large vessels"},
    "KAOHSIUNG": {"port_size": "Major", "locode": "TWKHH", "max_vessel_size": "large vessels"},
    "TANJUNG PELEPAS": {"port_size": "Major", "max_vessel_size": "large vessels"},
    "LAEM CHABANG": {"port_size": "Major", "max_vessel_size": "large vessels"},
    "HO CHI MINH": {"port_size": "Major", "locode": "VNSGN", "max_vessel_size": "large vessels"},
    "HAI PHONG": {"port_size": "Major", "locode": "VNHPH", "max_vessel_size": "large vessels"},
    "COLOMBO": {"port_size": "Major", "max_vessel_size": "large vessels"},
    "DUBAI": {"port_size": "Major", "locode": "AEJEA", "max_vessel_size": "large vessels"},
    "LONG BEACH": {"port_size": "Major", "max_vessel_size": "large vessels"},
    "VALENCIA": {"port_size": "Major", "locode": "ESVLC", "max_vessel_size": "large vessels"},
    "ALGECIRAS": {"port_size": "Major", "locode": "ESALG", "max_vessel_size": "large vessels"},
    "FELIXSTOWE": {"port_size": "Major", "locode": "GBFXT", "max_vessel_size": "large vessels"},
    "SAVANNAH": {"port_size": "Major", "locode": "USSAV", "max_vessel_size": "large vessels"},
    "MUNDRA": {"port_size": "Major", "locode": "INMUN", "max_vessel_size": "large vessels"},
    "LIANYUNGANG": {"port_size": "Major", "locode": "CNLYG", "max_vessel_size": "large vessels"},
    "TIANJIN": {"port_size": "Major", "max_vessel_size": "large vessels"},
    "TOKYO": {"port_size": "Major", "locode": "JPTYO", "max_vessel_size": "large vessels"},
    "BALBOA": {"port_size": "Major", "locode": "PABLB", "max_vessel_size": "large vessels"},
    "COLON": {"port_size": "Major", "locode": "PAMIT", "max_vessel_size": "large vessels"},
    "DAR ES SALAAM": {"port_size": "Large", "max_vessel_size": "large vessels"},
    "MOMBASA": {"port_size": "Large", "max_vessel_size": "large vessels"},
    "CHITTAGONG": {"port_size": "Large", "max_vessel_size": "large vessels"},
    "PIRAEUS": {"port_size": "Major", "locode": "GRPIR"},
    # Additional major ports
    "JIDDAH": {"port_size": "Major", "locode": "SAJED", "max_vessel_size": "large vessels"},
    "JAWAHARLAL NEHRU PORT (NHAVA SHIVA)": {"port_size": "Major", "max_vessel_size": "large vessels"},
    "TANGIER-MEDITERRANEAN": {"port_size": "Major", "locode": "MAPTM", "max_vessel_size": "large vessels"},
    "YANTIAN": {"port_size": "Major", "locode": "CNYTN", "max_vessel_size": "large vessels"},
    "SHEKOU": {"port_size": "Major", "max_vessel_size": "large vessels"},
}

def main():
    infile = "seaports.csv"
    rows = []
    with open(infile) as f:
        reader = csv.DictReader(f)
        fieldnames = reader.fieldnames
        for row in reader:
            rows.append(row)

    fixed = 0
    for row in rows:
        wpi = row["wpi_id"]
        name = row["name"]
        
        # Fix by WPI ID
        if wpi in FIXES:
            for k, v in FIXES[wpi].items():
                if k == "locode" and row.get("locode"):
                    continue  # Don't overwrite existing LOCODE
                row[k] = v
            fixed += 1
        
        # Fix by name
        if name in NAME_FIXES:
            for k, v in NAME_FIXES[name].items():
                if k == "locode" and row.get("locode"):
                    continue
                row[k] = v
            fixed += 1

    # Write back
    with open(infile, "w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(rows)
    
    print(f"Fixed {fixed} ports")

if __name__ == "__main__":
    main()
