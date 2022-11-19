import React from 'react';
import { Link, useLoaderData } from 'react-router-dom';
import { FixedSizeList } from 'react-window';


export async function traceLoader({ params }) {
    const response = await fetch(`/api/traces/${params.traceID}`);
    const traceData = await response.json();
    return traceData;
}

function Row({ index, style, data }) {
    return (
        <div className={index % 2 ? "ListItemOdd" : "ListItemEven"} style={style}>
            SpanID: {data[index].spanID} Name: {data[index].name}
        </div>
    );
}

export default function TraceView() {
    const { spans } = useLoaderData();
    return (
        <FixedSizeList
            className="List"
            height={300}
            itemData={spans}
            itemCount={spans.length}
            itemSize={50}
            width={"100%"}
        >
            {Row}
        </FixedSizeList>
    )
}