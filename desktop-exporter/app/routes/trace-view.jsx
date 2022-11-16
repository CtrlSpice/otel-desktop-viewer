import React from 'react';
import { useParams, Link } from 'react-router-dom';


export default function TraceView() {
    let { traceID } = useParams();
    return (
        <>
            <Link to={"/"}>Back</Link>
            <p>I am Item {traceID}</p>
        </>
    )
}