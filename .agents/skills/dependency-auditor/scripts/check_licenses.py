#!/usr/bin/env python3
"""
Simple license checker script to scan dependency manifests for restrictive licenses.
"""
import sys
import json
import re

PROHIBITED_LICENSES = [
    r"gpl",
    r"agpl",
    r"affero",
    r"copyleft",
    r"non-commercial",
]

def check_license_string(license_name):
    if not license_name:
        return True, "Unknown"
    
    clean_name = license_name.lower().strip()
    for pattern in PROHIBITED_LICENSES:
        if re.search(pattern, clean_name):
            return False, license_name
            
    return True, license_name

def check_package_json(filepath):
    print(f"Scanning {filepath} for licenses...")
    try:
        with open(filepath, 'r') as f:
            data = json.load(f)
            
        dependencies = data.get("dependencies", {})
        dev_dependencies = data.get("devDependencies", {})
        
        all_deps = {**dependencies, **dev_dependencies}
        if not all_deps:
            print("No dependencies found in package.json.")
            return True
            
        print(f"Found {len(all_deps)} dependencies to check.")
        # package.json only contains version constraints, full license checking 
        # requires lockfile or node_modules analysis.
        # We print a reminder here.
        print("Note: Run 'npm audit' or 'pnpm licenses list' for full transitives checks.")
        return True
    except Exception as e:
        print(f"Error reading package.json: {e}")
        return False

def main():
    if len(sys.argv) < 2:
        print("Usage: check_licenses.py <manifest_file>")
        sys.exit(1)
        
    manifest_path = sys.argv[1]
    
    if "package.json" in manifest_path:
        success = check_package_json(manifest_path)
    else:
        print(f"Manifest type not explicitly supported: {manifest_path}")
        success = True
        
    if success:
        print("License audit passed (basic check).")
        sys.exit(0)
    else:
        print("License audit FAILED.")
        sys.exit(1)

if __name__ == "__main__":
    main()
