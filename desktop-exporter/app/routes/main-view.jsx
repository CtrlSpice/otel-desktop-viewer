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

function useToggle(initialValue = false) {
    const [value, setValue] = React.useState(initialValue);
    const toggle = React.useCallback(() => {
        setValue((v) => !v);
    }, []);
    return [value, toggle];
}


function Sidebar(props) {
    if (props.isClosed) {
        return (
            <div className='sidebar closed'>
                <button className="menuBtn" onClick={props.toggle}>
                    Expand
                </button>
            </div>
        );
    }

    const { traceSummaries } = useLoaderData();
    return (
        <div className='sidebar'>
            <button className="menuBtn" onClick={props.toggle}>
                Collapse
            </button>
            <nav>
                <FixedSizeList
                    className="List"
                    height={500}
                    itemData={traceSummaries}
                    itemCount={traceSummaries.length}
                    itemSize={30}
                    width={"100%"}
                >
                    {Row}
                </FixedSizeList>
            </nav>
        </div>
    );

}

export default function MainView() {
    let [isClosed, toggleClosed] = useToggle();

    return (
        <div className='container'>
            <Sidebar isClosed={isClosed} toggle={toggleClosed} />
            <Outlet />
        </div>
    )
}