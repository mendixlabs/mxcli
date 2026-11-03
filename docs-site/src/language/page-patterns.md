# Common Patterns

This page collects frequently used page patterns: overview/list pages, edit pages, and master-detail layouts. Each pattern is a complete, working example.

## Overview / List Page

An overview page displays a list of entities with a control bar for creating, editing, and deleting records. Clicking a row opens the edit page.

```sql
CREATE PAGE MyModule.Customer_Overview FOLDER 'Customers'
(
  Title: 'Customers',
  Layout: Atlas_Core.Atlas_Default
)
{
  DATAGRID dgCustomers (DataSource: DATABASE MyModule.Customer, PageSize: 20, itemSelection: Single) {
    COLUMN (Attribute: Name, Caption: 'Name')
    COLUMN (Attribute: Email, Caption: 'Email')
    COLUMN (Attribute: Status, Caption: 'Status')
    COLUMN (Attribute: CreatedDate, Caption: 'Created')
    CONTROLBAR {
      ACTIONBUTTON btnNew (
        Caption: 'New Customer',
        Action: CALL MICROFLOW MyModule.ACT_Customer_New,
        ButtonStyle: Primary
      )
      ACTIONBUTTON btnEdit (Caption: 'Edit', Action: SHOW PAGE MyModule.Customer_Edit)
      ACTIONBUTTON btnDelete (Caption: 'Delete', Action: DELETE $dgCustomers, ButtonStyle: Danger)
    }
  }
};
```

The "New Customer" button calls a microflow that creates a new Customer object and opens the edit page:

```sql
CREATE MICROFLOW MyModule.ACT_Customer_New
BEGIN
  DECLARE $Customer MyModule.Customer;
  $Customer = CREATE MyModule.Customer (
    IsActive = true
  );
  SHOW PAGE MyModule.Customer_Edit (Customer = $Customer);
  RETURN $Customer;
END;
```

## Edit Page (Popup)

An edit page displayed as a popup dialog. Receives the entity as a page parameter:

```sql
CREATE PAGE MyModule.Customer_Edit FOLDER 'Customers'
(
  Params: ( $Customer: MyModule.Customer ),
  Title: 'Edit Customer',
  Layout: Atlas_Core.PopupLayout
)
{
  DATAVIEW dvCustomer (DataSource: $Customer) {
    TEXTBOX txtName (Label: 'Name', Attribute: Name)
    TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
    TEXTBOX txtPhone (Label: 'Phone', Attribute: Phone)
    COMBOBOX cbStatus (Label: 'Status', Attribute: Status)
    CHECKBOX cbActive (Label: 'Active', Attribute: IsActive)
    FOOTER {
      ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE CHANGES, ButtonStyle: Primary)
      ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL CHANGES)
    }
  }
};
```

## Detail Page (Full Page)

A full-page detail view with sections organized using layout grids:

```sql
CREATE PAGE MyModule.Customer_Detail FOLDER 'Customers'
(
  Params: ( $Customer: MyModule.Customer ),
  Title: 'Customer Detail',
  Layout: Atlas_Core.Atlas_Default
)
{
  DATAVIEW dvCustomer (DataSource: $Customer) {
    LAYOUTGRID grid1 {
      ROW {
        COLUMN {
          TEXTBOX txtName (Label: 'Name', Attribute: Name)
          TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
        }
        COLUMN {
          TEXTBOX txtPhone (Label: 'Phone', Attribute: Phone)
          COMBOBOX cbStatus (Label: 'Status', Attribute: Status)
        }
      }
      ROW {
        COLUMN {
          TEXTAREA txtNotes (Label: 'Notes', Attribute: Notes)
        }
      }
    }
    FOOTER {
      ACTIONBUTTON btnEdit (
        Caption: 'Edit',
        Action: SHOW PAGE MyModule.Customer_Edit,
        ButtonStyle: Primary
      )
      ACTIONBUTTON btnBack (Caption: 'Back', Action: CLOSE PAGE)
    }
  }
};
```

## Master-Detail Pattern

A master-detail page shows a list on one side and the selected item's details on the other. The SELECTION data source connects the detail view to the list.

### Side-by-Side Layout

```sql
CREATE PAGE MyModule.Product_MasterDetail FOLDER 'Products'
(
  Title: 'Products',
  Layout: Atlas_Core.Atlas_Default
)
{
  LAYOUTGRID gridMain {
    ROW {
      COLUMN {
        DATAGRID dgProducts (DataSource: DATABASE MyModule.Product, PageSize: 15) {
          COLUMN (Attribute: Name, Caption: 'Product')
          COLUMN (Attribute: Price, Caption: 'Price', Alignment: right)
          COLUMN (Attribute: Category, Caption: 'Category')
          CONTROLBAR {
            ACTIONBUTTON btnNew (
              Caption: 'New',
              Action: CALL MICROFLOW MyModule.ACT_Product_New,
              ButtonStyle: Primary
            )
          }
        }
      }
      COLUMN {
        DATAVIEW dvDetail (DataSource: SELECTION dgProducts) {
          TEXTBOX txtName (Label: 'Name', Attribute: Name)
          TEXTBOX txtDescription (Label: 'Description', Attribute: Description)
          TEXTBOX txtPrice (Label: 'Price', Attribute: Price)
          COMBOBOX cbCategory (Label: 'Category', Attribute: Category)
          FOOTER {
            ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE CHANGES, ButtonStyle: Primary)
          }
        }
      }
    }
  }
};
```

### With Related Items

A master-detail pattern where selecting an entity also shows its related child entities via an association:

```sql
CREATE PAGE MyModule.Order_MasterDetail FOLDER 'Orders'
(
  Title: 'Orders',
  Layout: Atlas_Core.Atlas_Default
)
{
  LAYOUTGRID gridMain {
    ROW {
      COLUMN {
        DATAGRID dgOrders (DataSource: DATABASE MyModule.Order, PageSize: 10) {
          COLUMN (Attribute: OrderId, Caption: 'Order #')
          COLUMN (Attribute: OrderDate, Caption: 'Date')
          COLUMN (Attribute: Status, Caption: 'Status')
          COLUMN (Attribute: TotalAmount, Caption: 'Total', Alignment: right)
        }
      }
      COLUMN {
        DATAVIEW dvOrder (DataSource: SELECTION dgOrders) {
          DYNAMICTEXT txtOrderInfo (Content: 'Order #{1}', Attribute: OrderId)
          TEXTBOX txtStatus (Label: 'Status', Attribute: Status)

          -- Order lines via association
          DATAGRID dgLines (DataSource: ASSOCIATION Order_OrderLine) {
            COLUMN (Attribute: ProductName, Caption: 'Product')
            COLUMN (Attribute: Quantity, Caption: 'Qty', Alignment: right)
            COLUMN (Attribute: LineTotal, Caption: 'Total', Alignment: right)
          }

          FOOTER {
            ACTIONBUTTON btnEdit (
              Caption: 'Edit Order',
              Action: SHOW PAGE MyModule.Order_Edit,
              ButtonStyle: Primary
            )
          }
        }
      }
    }
  }
};
```

## CRUD Page Set

A complete set of pages for managing an entity typically includes:

1. **Overview page** -- DataGrid with list of records + New/Edit/Delete buttons
2. **Edit page (popup)** -- DataView with input fields + Save/Cancel
3. **New microflow** -- Creates a blank object and opens the edit page

Here is a concise CRUD set for an `Employee` entity:

```sql
-- 1. Overview page
CREATE PAGE HR.Employee_Overview FOLDER 'Employees'
(
  Title: 'Employees',
  Layout: Atlas_Core.Atlas_Default
)
{
  DATAGRID dgEmployees (DataSource: DATABASE HR.Employee, PageSize: 20, itemSelection: Single) {
    COLUMN (Attribute: FullName, Caption: 'Name')
    COLUMN (Attribute: Department, Caption: 'Department')
    COLUMN (Attribute: HireDate, Caption: 'Hire Date')
    CONTROLBAR {
      ACTIONBUTTON btnNew (
        Caption: 'New Employee',
        Action: CALL MICROFLOW HR.ACT_Employee_New,
        ButtonStyle: Primary
      )
      ACTIONBUTTON btnEdit (Caption: 'Edit', Action: SHOW PAGE HR.Employee_Edit)
      ACTIONBUTTON btnDelete (Caption: 'Delete', Action: DELETE $dgEmployees, ButtonStyle: Danger)
    }
  }
};

-- 2. Edit page (popup)
CREATE PAGE HR.Employee_Edit FOLDER 'Employees'
(
  Params: ( $Employee: HR.Employee ),
  Title: 'Edit Employee',
  Layout: Atlas_Core.PopupLayout
)
{
  DATAVIEW dvEmployee (DataSource: $Employee) {
    TEXTBOX txtName (Label: 'Full Name', Attribute: FullName)
    TEXTBOX txtDept (Label: 'Department', Attribute: Department)
    DATEPICKER dpHireDate (Label: 'Hire Date', Attribute: HireDate)
    TEXTBOX txtEmail (Label: 'Email', Attribute: Email)
    FOOTER {
      ACTIONBUTTON btnSave (Caption: 'Save', Action: SAVE CHANGES, ButtonStyle: Primary)
      ACTIONBUTTON btnCancel (Caption: 'Cancel', Action: CANCEL CHANGES)
    }
  }
};

-- 3. New employee microflow
CREATE MICROFLOW HR.ACT_Employee_New
BEGIN
  DECLARE $Employee HR.Employee;
  $Employee = CREATE HR.Employee (
    HireDate = [%CurrentDateTime%]
  );
  SHOW PAGE HR.Employee_Edit (Employee = $Employee);
  RETURN $Employee;
END;
```

## Dashboard Page

A dashboard page using layout grids to arrange multiple data sections:

```sql
CREATE PAGE MyModule.Dashboard FOLDER 'Dashboard'
(
  Title: 'Dashboard',
  Layout: Atlas_Core.Atlas_Default
)
{
  LAYOUTGRID gridDash {
    ROW {
      COLUMN {
        CONTAINER cRecent (Class: 'card') {
          DYNAMICTEXT txtRecentTitle (Content: 'Recent Orders')
          DATAGRID dgRecent (DataSource: MICROFLOW MyModule.DS_RecentOrders, PageSize: 5) {
            COLUMN (Attribute: OrderId, Caption: 'Order #')
            COLUMN (Attribute: OrderDate, Caption: 'Date')
            COLUMN (Attribute: Status, Caption: 'Status')
          }
        }
      }
      COLUMN {
        CONTAINER cStats (Class: 'card') {
          DATAVIEW dvStats (DataSource: MICROFLOW MyModule.DS_GetStatistics) {
            DYNAMICTEXT txtTotal (Content: 'Total Orders: {1}', Attribute: TotalOrders)
            DYNAMICTEXT txtRevenue (Content: 'Revenue: ${1}', Attribute: TotalRevenue)
          }
        }
      }
    }
  }
};
```

## See Also

- [Pages](./pages.md) -- page overview
- [Page Structure](./page-structure.md) -- data sources and layout references
- [Widget Types](./widget-types.md) -- available widgets
- [ALTER PAGE](./alter-page.md) -- incremental modifications to existing pages
- [Snippets](./snippets.md) -- reusable page fragments
