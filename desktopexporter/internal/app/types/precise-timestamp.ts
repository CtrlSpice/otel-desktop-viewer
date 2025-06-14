export class PreciseTimestamp {
  constructor(
    public milliseconds: number,
    public nanoseconds: number
  ) {}

  static fromJSON(json: any): PreciseTimestamp {
    if (json instanceof PreciseTimestamp) {
      return json;
    }
    return new PreciseTimestamp(json.milliseconds, json.nanoseconds);
  }

  isBefore(other: PreciseTimestamp): boolean {
    return this.milliseconds < other.milliseconds || 
           (this.milliseconds === other.milliseconds && this.nanoseconds < other.nanoseconds);
  }

  isAfter(other: PreciseTimestamp): boolean {
    return this.milliseconds > other.milliseconds || 
           (this.milliseconds === other.milliseconds && this.nanoseconds > other.nanoseconds);
  }

  isEqual(other: PreciseTimestamp): boolean {
    return this.milliseconds === other.milliseconds && this.nanoseconds === other.nanoseconds;
  }

  toString(): string {
    const date = new Date(this.milliseconds);
    const year = date.getUTCFullYear();
    const month = String(date.getUTCMonth() + 1).padStart(2, '0');
    const day = String(date.getUTCDate()).padStart(2, '0');
    const hours = String(date.getUTCHours()).padStart(2, '0');
    const minutes = String(date.getUTCMinutes()).padStart(2, '0');
    const seconds = String(date.getUTCSeconds()).padStart(2, '0');
    const ms = String(date.getUTCMilliseconds()).padStart(3, '0');
    const ns = this.nanoseconds.toString().padStart(9, '0');
    return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}.${ms}${ns} +0000 UTC`;
  }
} 