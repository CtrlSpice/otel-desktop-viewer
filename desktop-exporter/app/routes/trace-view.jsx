import React from 'react';
import { useLoaderData } from 'react-router-dom';
import { FixedSizeList } from 'react-window';


export async function traceLoader({ params }) {
    const response = await fetch(`/api/traces/${params.traceID}`);
    const traceData = await response.json();
    return traceData;
}

function WaterfallRow({ index, style, data }) {
    let className = "waterfallItem"
    className += index % 2 ? " odd" : " even";
    className += index === data.activeIndex ? " active" : ""

    return (
        <div className={className} style={style} onClick={() => data.clickHandler(index)}>
            Name: {data.spans[index].name} SpanID: {data.spans[index].spanID} 
        </div>
    );
}

function Header(props) {
    return (
        <div className='header'>
            <h2>Trace ID: {props.traceID}</h2>
        </div>
    );
}

function WaterfallView(props) {
    return (
        <FixedSizeList
            className="List"
            height={300}
            itemData={props}
            itemCount={props.spans.length}
            itemSize={30}
            width={"100%"}
        >
            {WaterfallRow}
        </FixedSizeList>
    );
}

function DetailView(props) {
    return (
        <div className='detail'>
            Name: {props.spanData.name} <br />
            Kind: {props.spanData.kind} <br />
            Start: {props.spanData.startTime} <br />
            End: {props.spanData.endTime} <br />
        </div>
    );
}

export default function TraceView() {
    const traceData = useLoaderData();
    const [activeIndex, setActiveIndex] = React.useState(0);
    const handleOnClick = index => {
        setActiveIndex(index);
    };

    return (
        <div className="traceview">
            <Header traceID={traceData.traceID} />
            <WaterfallView spans={traceData.spans} clickHandler={handleOnClick} activeIndex={activeIndex} />
            <DetailView spanData={traceData.spans[activeIndex]} />
        </div>
    );
}