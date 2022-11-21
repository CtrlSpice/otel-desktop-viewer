import React from 'react';
import { Outlet, NavLink, useLoaderData } from "react-router-dom";
import { FixedSizeList } from 'react-window';

export async function mainLoader() {
    const response = await fetch("/api/traces");
    const traceSummaries = await response.json();
    return traceSummaries;
}


function Row({ index, style, data }) {
    return (
        <NavLink to={`traces/${data[index].traceID}`} style={style}>
            {data[index].traceID}
        </NavLink>
    );
}

export default function MainView() {
    const { traceSummaries } = useLoaderData();
    return (
        <div className='wrapper'>
            <div className='sidebar'>
                <button>Refresh</button>
                <button>Collapse</button>
                <nav>
                    <FixedSizeList
                        className="List"
                        height={300}
                        itemData={traceSummaries}
                        itemCount={traceSummaries.length}
                        itemSize={50}
                        width={"100%"}
                    >
                        {Row}
                    </FixedSizeList>
                </nav>
            </div>

            <div className='header'>
                <h1>Hello, I'm Heather!</h1>
            </div>
            <div className="traceview">
                <Outlet />
            </div>
            <div className='detail'>
                Span details live here.
            </div>
        </div>
    )
}