export class PreciseTimestamp {
  constructor(nanoseconds: bigint) {
    this.nanoseconds = nanoseconds;
  }

  nanoseconds: bigint;

  static fromJSON(
    json: { milliseconds: number; nanoseconds: number } | string
  ): PreciseTimestamp {
    // Handle string format from backend (nanoseconds as string)
    if (typeof json === 'string') {
      return new PreciseTimestamp(BigInt(json));
    }

    // Handle object format (legacy or alternative format)
    if (
      json &&
      typeof json === 'object' &&
      json.milliseconds !== undefined &&
      json.nanoseconds !== undefined
    ) {
      return new PreciseTimestamp(
        BigInt(json.milliseconds) * BigInt(1_000_000) + BigInt(json.nanoseconds)
      );
    }

    // Handle undefined/null case
    throw new Error(`Invalid timestamp format: ${JSON.stringify(json)}`);
  }

  toUTC(): string {
    let totalMs = this.nanoseconds / BigInt(1_000_000);
    let remainderNs = this.nanoseconds % BigInt(1_000_000);
    let date = new Date(Number(totalMs));
    let year = date.getUTCFullYear();
    let month = String(date.getUTCMonth() + 1).padStart(2, '0');
    let day = String(date.getUTCDate()).padStart(2, '0');
    let hours = String(date.getUTCHours()).padStart(2, '0');
    let minutes = String(date.getUTCMinutes()).padStart(2, '0');
    let seconds = String(date.getUTCSeconds()).padStart(2, '0');
    let ms = String(date.getUTCMilliseconds()).padStart(3, '0');
    let ns = remainderNs.toString().padStart(9, '0');
    return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}.${ms}${ns} +0000 UTC`;
  }

  toLocal(): string {
    let totalMs = this.nanoseconds / BigInt(1_000_000);
    let remainderNs = this.nanoseconds % BigInt(1_000_000);
    let date = new Date(Number(totalMs));
    let year = date.getFullYear();
    let month = String(date.getMonth() + 1).padStart(2, '0');
    let day = String(date.getDate()).padStart(2, '0');
    let hours = String(date.getHours()).padStart(2, '0');
    let minutes = String(date.getMinutes()).padStart(2, '0');
    let seconds = String(date.getSeconds()).padStart(2, '0');
    let ms = String(date.getMilliseconds()).padStart(3, '0');
    let ns = remainderNs.toString().padStart(9, '0');
    let timezone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}.${ms}${ns} ${timezone}`;
  }

  toString(): string {
    return this.toLocal();
  }
}
