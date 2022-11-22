import React from 'react';
import { Link, useLoaderData, useOutletContext } from 'react-router-dom';
import { FixedSizeList } from 'react-window';


export async function traceLoader({ params }) {
    const response = await fetch(`/api/traces/${params.traceID}`);
    const traceData = await response.json();
    return traceData;
}

function WaterfallRow({ index, style, data }) {
    return (
        <div className={index % 2 ? "waterfall odd" : "waterfall even"} style={style}>
            Name: {data[index].name} SpanID: {data[index].spanID} 
        </div>
    );
}

function Header(props) {
    return (
        <div className='header'>
            <h1>Trace ID: {props.traceID}</h1>
        </div>
    );
}

function WaterfallView(props) {
    return (
        <FixedSizeList
            className="List"
            height={300}
            itemData={props.spans}
            itemCount={props.spans.length}
            itemSize={30}
            width={"100%"}
        >
            {WaterfallRow}
        </FixedSizeList>
    );
}

function DetailView() {
    return (
        <div className='detail'>
            Details will go here
        </div>
    )
}

export default function TraceView() {
    const traceData = useLoaderData();

    return (
        <div className="traceview">
            <Header traceID={traceData.traceID} />
            <WaterfallView spans={traceData.spans} />
            <DetailView />
        </div>
    );
}