import React from "react";
import { Outlet, NavLink, useLoaderData } from "react-router-dom";
import { FixedSizeList } from "react-window";
import { useToggle } from "usehooks-ts";

export async function mainLoader() {
  const response = await fetch("/api/traces");
  const traceSummaries = await response.json();
  return traceSummaries;
}

function Row({ index, style, data }) {
  return (
    <NavLink
      to={`traces/${data[index].traceID}`}
      style={style}
    >
      {data[index].traceID}
    </NavLink>
  );
}

function Sidebar(props) {
  if (props.isClosed) {
    return (
      <div className="sidebar closed">
        <button
          className="menuBtn"
          onClick={props.toggle}
        >
          Expand
        </button>
      </div>
    );
  }

  const { traceSummaries } = useLoaderData();
  return (
    <div className="sidebar">
      <button
        className="menuBtn"
        onClick={props.toggle}
      >
        Collapse
      </button>
      <nav>
        <FixedSizeList
          className="list"
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
    <div className="container">
      <Sidebar
        isClosed={isClosed}
        toggle={toggleClosed}
      />
      <Outlet />
    </div>
  );
}
