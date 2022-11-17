import React from 'react';
import { Link, useLoaderData } from "react-router-dom";

export async function loader() {
    const response = await fetch("/traces");
    const traceSummaries = await response.json();
    return traceSummaries;
}

export default function MainView() {
    const { traceSummaries } = useLoaderData();
    console.log(traceSummaries);

    const summaries = traceSummaries.map((summary) => (
        <li key={summary.traceID}>
            <Link to={`traces/${summary.traceID}`}>
                Trace: {summary.traceID}
            </Link>
        </li>
    ));

    return (
        <>
            {traceSummaries.length ? (
                <ul>{summaries}</ul>
            ) : (
                <p><i>No traces</i></p>
            )}
        </>
    )
}