import React from 'react';
import { Link } from "react-router-dom";

export default function MainView() {
    return (
        <>
            <ul>
                <li>
                    <Link to={"traces/1"}>Item 1</Link>
                </li>
                <li>
                    <Link to={"traces/2"}>Item 2</Link>
                </li>
                <li>
                    <Link to={"traces/3"}>Item 3</Link>
                </li>
            </ul>
        </>
    )
}