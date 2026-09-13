#!/usr/bin/env bash
# PenguinDB Colorized Differential SQL Test Runner (check.sh)
set -euo pipefail

# ANSI Color Codes
BOLD_CYAN='\033[1;36m'
BOLD_GREEN='\033[1;32m'
BOLD_RED='\033[1;31m'
BOLD_YELLOW='\033[1;33m'
BOLD_MAGENTA='\033[1;35m'
COLOR_RED='\033[0;31m'
COLOR_GREEN='\033[0;32m'
COLOR_CYAN='\033[0;36m'
NC='\033[0m' # No Color

PG_HOST="${PGHOST:-127.0.0.1}"
PG_PORT="${PGPORT:-5432}"
PG_USER="${PGUSER:-postgres}"
PG_DB="${PGDATABASE:-postgres}"

PENG_HOST="${PENGHOST:-127.0.0.1}"
PENG_PORT="${PENGPORT:-5433}"

SUITE_DIR="tests/sql_suite"

echo -e "${BOLD_CYAN}=======================================================${NC}"
echo -e "${BOLD_CYAN}   PenguinDB Colorized Differential SQL Test Suite     ${NC}"
echo -e "${BOLD_CYAN}=======================================================${NC}"

# Clean previous temporary test artifacts (.pengout and .diff)
rm -f "${SUITE_DIR}"/*.pengout "${SUITE_DIR}"/*.diff 2>/dev/null || true

PASSED=0
FAILED=0
TOTAL=0
IDX=0

for sql_file in "${SUITE_DIR}"/*.sql; do
    [ -e "${sql_file}" ] || continue
    IDX=$((IDX + 1))
    TOTAL=$((TOTAL + 1))
    test_name=$(basename "${sql_file}" .sql)
    out_file="${SUITE_DIR}/${test_name}.out"
    pengout_file="${SUITE_DIR}/${test_name}.pengout"
    diff_file="${SUITE_DIR}/${test_name}.diff"

    echo -ne "Test ${IDX}: ${BOLD_YELLOW}${test_name}${NC} ... "

    # Step 1: Run psql with CSV output formatting if psql exists (STDOUT ONLY 2>/dev/null, quiet mode -q)
    if command -v psql >/dev/null 2>&1; then
        PGPASSWORD="${PGPASSWORD:-postgres}" psql -q -h "${PG_HOST}" -p "${PG_PORT}" -U "${PG_USER}" -d "${PG_DB}" -X -P format=csv -f "${sql_file}" 2>/dev/null | grep -v -E '^(CREATE|DROP|INSERT|UPDATE|DELETE|SET|USE|database|NOTICE|WARNING|You are now connected)' > "${out_file}" || true
    elif [ ! -f "${out_file}" ]; then
        echo -ne "${BOLD_YELLOW}[CREATING BASELINE .out] ${NC}"
        go run ./tests/sql_suite/main.go -file "${sql_file}" -out "${out_file}" -host "${PENG_HOST}" -port "${PENG_PORT}"
    fi

    # Step 2: Run Go pengrunner CLI to generate CSV .pengout (tabular SELECT queries only)
    go run ./tests/sql_suite/main.go -file "${sql_file}" -out "${pengout_file}" -host "${PENG_HOST}" -port "${PENG_PORT}"

    # Step 3: Compare .out and .pengout CSVs with canonical rowsort for result blocks
    norm_out="${SUITE_DIR}/${test_name}.out.norm"
    norm_peng="${SUITE_DIR}/${test_name}.pengout.norm"
    
    awk '
    /^(ERROR:|id|count|min|max|sum|avg|category|country|status|genre|dept_|building|floor|severity|course_|grade|author_|first_|project_|monthly_|plan|flight_|book_|borrower_|returned|location_|device_|sensor_|reading_)/ {
        if (block != "") {
            split(block, lines, "\n")
            asort(lines)
            for (i in lines) if (lines[i] != "") print lines[i]
            block = ""
        }
        print $0
        next
    }
    { block = block "\n" $0 }
    END {
        if (block != "") {
            split(block, lines, "\n")
            asort(lines)
            for (i in lines) if (lines[i] != "") print lines[i]
        }
    }' "${out_file}" > "${norm_out}" || cp "${out_file}" "${norm_out}"

    awk '
    /^(ERROR:|id|count|min|max|sum|avg|category|country|status|genre|dept_|building|floor|severity|course_|grade|author_|first_|project_|monthly_|plan|flight_|book_|borrower_|returned|location_|device_|sensor_|reading_)/ {
        if (block != "") {
            split(block, lines, "\n")
            asort(lines)
            for (i in lines) if (lines[i] != "") print lines[i]
            block = ""
        }
        print $0
        next
    }
    { block = block "\n" $0 }
    END {
        if (block != "") {
            split(block, lines, "\n")
            asort(lines)
            for (i in lines) if (lines[i] != "") print lines[i]
        }
    }' "${pengout_file}" > "${norm_peng}" || cp "${pengout_file}" "${norm_peng}"

    if diff -u -a "${norm_out}" "${norm_peng}" > "${diff_file}"; then
        echo -e "${BOLD_GREEN}[PASS]${NC}"
        rm -f "${diff_file}" "${norm_out}" "${norm_peng}"
        PASSED=$((PASSED + 1))
    else
        echo -e "${BOLD_RED}[FAIL]${NC}"
        FAILED=$((FAILED + 1))
        echo -e "${BOLD_MAGENTA}--- Diff Output for Test ${IDX} (${test_name}) ---${NC}"
        while IFS= read -r line; do
            if [[ $line =~ ^--- ]] || [[ $line =~ ^\+\+\+ ]]; then
                echo -e "${BOLD_MAGENTA}${line}${NC}"
            elif [[ $line =~ ^@@ ]]; then
                echo -e "${COLOR_CYAN}${line}${NC}"
            elif [[ $line =~ ^- ]]; then
                echo -e "${COLOR_RED}${line}${NC}"
            elif [[ $line =~ ^\+ ]]; then
                echo -e "${COLOR_GREEN}${line}${NC}"
            else
                echo "${line}"
            fi
        done < "${diff_file}"
        echo -e "${BOLD_MAGENTA}------------------------------------------------${NC}"
    fi
done

echo -e "${BOLD_CYAN}=======================================================${NC}"
echo -e "Test Results: ${BOLD_GREEN}${PASSED} Passed${NC}, ${BOLD_RED}${FAILED} Failed${NC}, ${BOLD_YELLOW}${TOTAL} Total${NC}"
echo -e "${BOLD_CYAN}=======================================================${NC}"

if [ "${FAILED}" -gt 0 ]; then
    exit 1
fi
