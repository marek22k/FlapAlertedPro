window.addEventListener('load', function () {
    display();
});

function display() {
    fetch("../../flaps/active").then(function (response) {
        return response.json();
    }).then(function (json) {
        let pJson;
        let prefix = new URL(location.href).searchParams.get("prefix");
        if (prefix == null) {
            alert("Invalid link");
            return;
        }

        for (let i = 0; i < json.length; i++) {
            if (json[i].Prefix === prefix) {
                pJson = json[i].Paths
                break;
            }
        }

        if (pJson == null) {
            alert("Prefix not found");
            return;
        }

        if (pJson.length === 0) {
            alert("The analysis feature is not available as the instance has been configured to not keep path information")
        }

        let obj = [];
        for (let i = 0; i < pJson.length; i++) {
            let firstAsn = pJson[i].Asn[0];

            if (obj[firstAsn] == null) {
                obj[firstAsn] = [pJson[i].Asn];
            } else {
                obj[firstAsn].push(pJson[i].Asn);
            }
        }

        let tableHtml = document.createElement("span");

        for (const key in obj) {
            for (let c = 0; c < obj[key].length; c++) {
                for (let d = 0; d < obj[key][c].length; d++) {
                    let sa = obj[key][c][d].toString();
                    let saLen = sa.length;
                    let gap = " ";
                    while (saLen < 10) {
                        gap = gap + "&nbsp;";
                        saLen++;
                    }
                    let hexColor = stringToColor(sa);
                    let r = parseInt(hexColor.slice(1, 3), 16);
                    let g = parseInt(hexColor.slice(3, 5), 16);
                    let b = parseInt(hexColor.slice(5, 7), 16);
                    let span = document.createElement("span");
                    span.style.backgroundColor = "rgba(" + r + "," + g + "," + b + "," + "0.3')";
                    span.innerHTML = gap + sa;
                    tableHtml.appendChild(span);
                }
                tableHtml.appendChild(document.createElement("br"));
            }
            tableHtml.appendChild(document.createElement("br"));
        }
        document.getElementById("pathTable").replaceChildren(tableHtml);
        document.getElementById("prefixTitle").innerHTML = "Flap analysis for " + prefix;
        document.getElementById("loader").style.display = "none";
        document.getElementById("loaderText").style.display = "none";
    }).catch(function () {
        alert("Network error");
    });
}


function stringToColor(str) {
    let hash = 0;
    for (let i = 0; i < str.length; i++) {
        hash = str.charCodeAt(i) + ((hash << 5) - hash);
    }
    let colour = '#';
    for (let i = 0; i < 3; i++) {
        let value = (hash >> (i * 8)) & 0xFF;
        let rawColour = '00' + value.toString(16);
        colour += rawColour.substring(rawColour.length-2);
    }
    return colour;
}
