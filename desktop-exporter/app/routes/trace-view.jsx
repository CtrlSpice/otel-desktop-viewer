import React from 'react';
import { useParams, Link, useLoaderData } from 'react-router-dom';

export async function traceLoader({ params }) {
    const response = await fetch(`/traces/${params.traceID}`);
    const traceData = await response.json();
    return traceData;
}


export default function TraceView() {
    const { spans } = useLoaderData();
    console.log(spans);
    return (
        <>
            <Link to={"/"}>Back</Link>
            <p>I am Item {spans[0].traceID}</p>
        </>
    )
}