import React from 'react';
import { useParams } from 'react-router-dom';

export default function TraceView() {
    let { traceID } = useParams();
    return (
        <>
            <a href="/">Back</a>
            <p>I am Item {traceID}</p>
        </>
    )
}