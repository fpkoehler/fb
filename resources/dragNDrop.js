var dragItem;
var active = false;
var currentX;
var currentY;
var initialX;
var initialY;
var xOffset = 0;
var yOffset = 0;

var selectTable = document.getElementById('selectTable');

var tableRows = document.querySelectorAll('tr.draggable');
tableRows.forEach(function(item) {
  item.addEventListener("touchstart", dragStart, false);
  item.addEventListener("touchend", dragEnd, false);
  item.addEventListener("touchmove", drag, false);
  item.addEventListener("mousedown", dragStart, false);
  item.addEventListener("mouseup", dragEnd, false);
  item.addEventListener("mousemove", drag, false);
});

function dragStart(e) {
  if (this.rowIndex == 0) {
    console.log("row 0 selected, exiting drawStart");
    return;
  }
  var cursorX;
  var cursorY;
  if (e.type === "touchstart") {
    initialX = e.touches[0].clientX - xOffset;
    initialY = e.touches[0].clientY - yOffset;
    cursorX = e.touches[0].clientX;
    cursorY = e.touches[0].clientY;
  } else {
    initialX = e.clientX - xOffset;
    initialY = e.clientY - yOffset;
    cursorX = e.clientX;
    cursorY = e.clientY;
  }
  offsetXRadioCell = this.children[2].offsetLeft;
  if (cursorX > offsetXRadioCell) {
    // Can not drag from the cell with the radio selects
    // because there is confusion between drag vs clicking
    // a radio selector
    console.log("x > offsetXRadioCell");
    return;
  }
  dragItem = this;
  active = true;
}

function dragEnd(e) {
  if (!active) {
    return;
  }
  active = false;

  var destRow = getHoverElement(currentX+initialX, currentY+initialY);

  xOffset = 0;
  yOffset = 0;
  dragItem.style.transform = "none"

  if (typeof dragItem.rowIndex === "undefined") {
    // not over a row, so don't do anything.
    console.log("dragEnd: not over a row");
  } else {
    // row 0 is the header
    if (destRow.rowIndex != 0) {
      moveRowEntry(dragItem.rowIndex, destRow.rowIndex);
    } else {
      console.log("dragEnd: over row 0");
    }
  }
}

function drag(e) {
  if (active) {

    e.preventDefault();

    var cursorX;
    var cursorY;
    if (e.type === "touchmove") {
      currentX = e.touches[0].clientX - initialX;
      currentY = e.touches[0].clientY - initialY;
      cursorX = e.touches[0].clientX;
      cursorY = e.touches[0].clientY;
    } else {
      currentX = e.clientX - initialX;
      currentY = e.clientY - initialY;
      cursorX = e.clientX;
      cursorY = e.clientY;
    }

    // Offset represents distance from initial position.
    xOffset = currentX;
    yOffset = currentY;

    // Translate is from the original position, not from current position.
    //setTranslate(currentX, currentY, dragItem);
    setTranslate(0, currentY, dragItem);
  }
}

function setTranslate(xPos, yPos, el) {
  el.style.transform = "translate3d(" + xPos + "px, " + yPos + "px, 0)";
}

function getHoverElement(x,y){
  var element = "none";

  x += window.pageXOffset;
  y += window.pageYOffset;
  tableRows.forEach(function(item) {
    var el_left = item.offsetLeft + selectTable.offsetLeft;
    var el_right= el_left + item.offsetWidth;
    var el_top = item.offsetTop + selectTable.offsetTop;
    var el_bottom = el_top + item.offsetHeight;
    if (x >= el_left && x <= el_right) {
      if (y >= el_top && y <= el_bottom) {
        if (dragItem.rowIndex != item.rowIndex) {
          element = item;
          return false;
        }
      }
    }
  });
  return element;
}

function moveRowEntry(sIndex, tIndex) {
  var i;
  var rows = selectTable.getElementsByTagName("TR");

  if (tIndex > sIndex) {
      /* bubble down */
      for (i = sIndex; i < tIndex; i++) {
          rows[i].parentNode.insertBefore(rows[i+1], rows[i]);
          var tmp = rows[i].cells[0].children[0].value;
          rows[i].cells[0].children[0].value = rows[i+1].cells[0].children[0].value;
          rows[i+1].cells[0].children[0].value = tmp;
      }
  }
  if (sIndex > tIndex) {
      /* bubble up */
      for (i = sIndex-1; i >= tIndex; i--) {
          rows[i].parentNode.insertBefore(rows[i+1], rows[i]);
          var tmp = rows[i].cells[0].children[0].value;
          rows[i].cells[0].children[0].value = rows[i+1].cells[0].children[0].value;
          rows[i+1].cells[0].children[0].value = tmp;
      }
  }
}
