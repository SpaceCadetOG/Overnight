#!/usr/bin/env python3

import argparse
import csv
import io
import sys
import time
import urllib.error
import urllib.request
import zipfile
from datetime import datetime, timezone
from pathlib import Path


def months(start: str, end: str):
    current = datetime.strptime(start, "%Y-%m-%d")
    exclusive = datetime.strptime(end, "%Y-%m-%d")
    while current < exclusive:
        yield current.strftime("%Y-%m")
        if current.month == 12:
            current = current.replace(year=current.year + 1, month=1)
        else:
            current = current.replace(month=current.month + 1)


def timestamp(milliseconds: str) -> str:
    value = datetime.fromtimestamp(int(milliseconds) / 1000, timezone.utc)
    if int(milliseconds) % 1000:
        return value.isoformat(timespec="milliseconds").replace("+00:00", "Z")
    return value.isoformat(timespec="seconds").replace("+00:00", "Z")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--symbol", required=True)
    parser.add_argument("--interval", default="5m")
    parser.add_argument("--start", default="2022-01-01")
    parser.add_argument("--end", default="2026-08-01")
    parser.add_argument("--output", required=True)
    args = parser.parse_args()

    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = output.with_suffix(output.suffix + ".partial")
    rows_written = 0

    with temporary.open("w", newline="") as destination:
        writer = csv.writer(destination)
        writer.writerow(["open_time", "close_time", "open", "high", "low", "close", "volume"])

        for month in months(args.start, args.end):
            filename = f"{args.symbol}-{args.interval}-{month}.zip"
            url = (
                "https://data.binance.vision/data/futures/um/monthly/klines/"
                f"{args.symbol}/{args.interval}/{filename}"
            )

            archive_data = None
            for attempt in range(1, 6):
                try:
                    with urllib.request.urlopen(url, timeout=60) as response:
                        archive_data = response.read()
                    break
                except urllib.error.HTTPError as error:
                    if error.code == 404:
                        print(f"{args.symbol} {month}: unavailable", file=sys.stderr)
                        break
                    if attempt == 5:
                        raise
                except (urllib.error.URLError, TimeoutError):
                    if attempt == 5:
                        raise
                time.sleep(attempt * 2)

            if archive_data is None:
                continue

            with zipfile.ZipFile(io.BytesIO(archive_data)) as archive:
                csv_name = archive.namelist()[0]
                with archive.open(csv_name) as raw:
                    reader = csv.reader(io.TextIOWrapper(raw, encoding="utf-8"))
                    month_rows = 0
                    for row in reader:
                        if not row or not row[0].isdigit():
                            continue
                        writer.writerow([
                            timestamp(row[0]),
                            timestamp(row[6]),
                            row[1],
                            row[2],
                            row[3],
                            row[4],
                            row[5],
                        ])
                        rows_written += 1
                        month_rows += 1

            print(f"{args.symbol} {month}: {month_rows} candles", file=sys.stderr)

    if rows_written == 0:
        temporary.unlink(missing_ok=True)
        raise RuntimeError(f"no candles downloaded for {args.symbol}")

    temporary.replace(output)
    print(f"Saved {rows_written} candles to {output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
