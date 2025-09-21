export type Timezone = 'local' | 'UTC';

type TimeResolution =
  | 'minutes' // 2024-01-15 14:30
  | 'seconds' // 2024-01-15 14:30:45
  | 'milliseconds' // 2024-01-15 14:30:45.123
  | 'microseconds' // 2024-01-15 14:30:45.123456
  | 'nanoseconds'; // 2024-01-15 14:30:45.123456789

// Function overloads
export function formatDateTime(
  timestamp: number | bigint,
  timezone: Timezone,
  resolution: 'minutes' | 'seconds' | 'milliseconds'
): string;
export function formatDateTime(
  timestamp: bigint,
  timezone: Timezone,
  resolution: 'microseconds' | 'nanoseconds'
): string;

// Implementation
export function formatDateTime(
  timestamp: number | bigint,
  timezone: Timezone,
  resolution: TimeResolution = 'minutes'
): string {
  // function body
  // Base format options
  let options: Intl.DateTimeFormatOptions = {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  };

  let date: Date;
  let remainder: bigint | undefined;
  let formattedDate: string;

  if (typeof timestamp === 'bigint') {
    if (resolution === 'microseconds') {
      // Extract milliseconds and keep remainder for microseconds
      let divisor = 1000000n;
      let milliseconds = Number(timestamp / divisor);
      remainder = timestamp % divisor;
      date = new Date(milliseconds);
    } else if (resolution === 'nanoseconds') {
      // Extract milliseconds and keep remainder for nanoseconds
      let divisor = 1000000000n;
      let milliseconds = Number(timestamp / divisor);
      remainder = timestamp % divisor;
      date = new Date(milliseconds);
    } else {
      // For lower resolutions, just convert to milliseconds
      date = new Date(Number(timestamp));
    }
  } else {
    date = new Date(timestamp);
  }

  // Add resolution-specific options
  switch (resolution) {
    case 'seconds':
      // Format: 2024-01-15 14:30:45
      options.second = '2-digit';
      break;
    case 'milliseconds':
    case 'microseconds':
    case 'nanoseconds':
      // Format: 2024-01-15 14:30:45.123 (milli), .123456 (micro), .123456789 (nano)
      options.second = '2-digit';
      options.fractionalSecondDigits = 3;
  }

  if (timezone === 'UTC') {
    // Display in UTC using en-CA locale
    formattedDate = date.toLocaleString('en-CA', {
      ...options,
      timeZone: 'UTC',
    });
  } else {
    // Display in local timezone using en-CA locale
    formattedDate = date.toLocaleString('en-CA', options);
  }

  // Handle microseconds and nanoseconds manually
  if (resolution === 'microseconds' || resolution === 'nanoseconds') {
    let fractionalPart = '';

    if (resolution === 'microseconds') {
      fractionalPart = Number(remainder!).toString().padStart(6, '0');
    } else if (resolution === 'nanoseconds') {
      fractionalPart = Number(remainder!).toString().padStart(9, '0');
    }

    // Replace the last 3 digits with our extended precision
    formattedDate = formattedDate.replace(/\.\d{3}$/, `.${fractionalPart}`);
  }

  // Add timezone info
  if (timezone === 'UTC') {
    return `${formattedDate} UTC`;
  } else {
    // Get local timezone abbreviation
    let tzAbbr =
      new Intl.DateTimeFormat('en', {
        timeZoneName: 'short',
      })
        .formatToParts(date)
        .find(part => part.type === 'timeZoneName')?.value || '';

    return `${formattedDate} ${tzAbbr}`;
  }
}

export function formatDateTimeRange(
  start: number,
  end: number,
  timezone: Timezone
): string {
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
