export type Timezone = 'local' | 'UTC';

type DateTimeResolution = 'minutes' | 'seconds' | 'milliseconds';
type TimestampResolution = DateTimeResolution | 'microseconds' | 'nanoseconds';

// UI time: number = Unix ms (from Date.now(), time pickers, etc.)
export function formatDateTime(
  ms: number,
  timezone: Timezone,
  resolution: DateTimeResolution = 'minutes'
): string {
  return formatWithDate(new Date(ms), timezone, resolution);
}

// Telemetry time: bigint = Unix nanoseconds (from backend OTLP data)
export function formatTimestamp(
  ns: bigint,
  timezone: Timezone,
  resolution: TimestampResolution = 'nanoseconds'
): string {
  let epochMs = Number(ns / 1_000_000n);
  let subMs = ns % 1_000_000n;
  let date = new Date(epochMs);
  let formatted = formatWithDate(date, timezone, resolution);

  if (resolution === 'microseconds') {
    let micros = Number(subMs).toString().padStart(6, '0');
    return formatted.replace(/\.\d{3}(\s)/, `.${micros}$1`);
  }
  if (resolution === 'nanoseconds') {
    let nanos = Number(subMs).toString().padStart(6, '0');
    let extraNanos = Number(ns % 1000n).toString().padStart(3, '0');
    return formatted.replace(/\.\d{3}(\s)/, `.${nanos}${extraNanos}$1`);
  }
  return formatted;
}

function formatWithDate(
  date: Date,
  timezone: Timezone,
  resolution: TimestampResolution
): string {
  let options: Intl.DateTimeFormatOptions = {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  };

  switch (resolution) {
    case 'seconds':
      options.second = '2-digit';
      break;
    case 'milliseconds':
    case 'microseconds':
    case 'nanoseconds':
      options.second = '2-digit';
      options.fractionalSecondDigits = 3;
      break;
  }

  let formattedDate: string;
  if (timezone === 'UTC') {
    formattedDate = date.toLocaleString('en-CA', { ...options, timeZone: 'UTC' });
  } else {
    formattedDate = date.toLocaleString('en-CA', options);
  }

  if (timezone === 'UTC') {
    return `${formattedDate} UTC`;
  }

  let tzAbbr =
    new Intl.DateTimeFormat('en', { timeZoneName: 'short' })
      .formatToParts(date)
      .find(part => part.type === 'timeZoneName')?.value || '';
  return `${formattedDate} ${tzAbbr}`;
}

export function formatDateTimeRange(
  start: number,
  end: number,
  timezone: Timezone
): string {
  // Handle "Show all" case where start is 0 (beginning of time)
  if (start === 0) {
    return `Before ${formatDateTime(end, timezone, 'seconds')}`;
  }

  let startStr = formatDateTime(start, timezone, 'seconds');
  let endStr = formatDateTime(end, timezone, 'seconds');

  // Extract date and time parts for reuse
  let startParts = startStr.split(' ');
  let endParts = endStr.split(' ');
  let timezoneSuffix = startParts[2] ?? '';
  let isSameDay = startParts[0] === endParts[0];

  startStr = startStr.replace(timezoneSuffix, '');
  endStr = endStr.replace(timezoneSuffix, '');

  if (isSameDay) {
    // Same day: "2024-01-15 14:30:45 - 15:45:30 UTC"
    return `${startStr} - ${endParts[1]} ${timezoneSuffix}`;
  } else {
    // Different days: "2024-01-15 14:30:45 - 2024-01-16 09:15:30 UTC"
    return `${startStr} - ${endStr} ${timezoneSuffix}`;
  }
}

export function getLocalTimezoneName(): string {
  try {
    // Get the local timezone name (e.g., "Pacific Standard Time", "Eastern Daylight Time")
    let timeZoneName = new Intl.DateTimeFormat('en', {
      timeZoneName: 'long',
    })
      .formatToParts(new Date())
      .find(part => part.type === 'timeZoneName')?.value;

    return timeZoneName || 'Local Time';
  } catch (error) {
    return 'Local Time';
  }
}
