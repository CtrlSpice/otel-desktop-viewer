import React from 'react';
import { Link, useLoaderData } from "react-router-dom";

export async function mainLoader() {
    const response = await fetch("/traces");
    const traceSummaries = await response.json();
    return traceSummaries;
}

export default function MainView() {
    const { traceSummaries } = useLoaderData();
    console.log(traceSummaries);

    const summaries = traceSummaries.map((summary) => (
        <tr key={summary.traceID}>
            <td><Link to={`traces/${summary.traceID}`}>{summary.traceID}</Link></td>
            <td>{summary.spanCount}</td>
            <td>{summary.durationMS}</td>
        </tr>
    ));

    return (
        <>
            {traceSummaries.length ? (
                <table>
                    <thead>
                        <tr>
                            <th>Trace ID</th>
                            <th>Span Count</th>
                            <th>Duration (MS)</th>
                        </tr>
                    </thead>
                    <tbody>
                        {summaries}
                    </tbody>
                </table>

            ) : (
                    <p>No traces yet</p>
            )}
        </>
    )
}