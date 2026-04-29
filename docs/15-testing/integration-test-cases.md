# Integration Test Cases — Manual Testing

**Updated:** 2026-04-29
**PR:** [mendixlabs/mxcli#301](https://github.com/mendixlabs/mxcli/pull/301)

Covers: OData Clients, OData Services, External Entities, Published REST Services, REST Clients, JSON Structures.

## Test Projects

Demo apps from [Mendix App Gallery](https://appgallery.mendixcloud.com/):

| App | Studio Pro | Integration features |
|-----|-----------|----------------------|
| Lato Enquiry Management | 11.4.0 | OData, REST, JSON structures |
| Evora - Factory Management | 10.24.15 | OData services, external entities |
| Lato Product Inventory | 11.2.0 | Published REST, consumed OData |

---

## Setup

### 1. Download test apps

1. Go to [Mendix App Gallery](https://appgallery.mendixcloud.com/)
2. Download each demo app listed above
3. Open each `.mpk` in Studio Pro to extract the `.mpr` file

### 2. Build mxcli

```bash
make build && make test && make lint-go
```

### 3. Smoke test

```bash
APPS_DIR=<path-to-extracted-apps>
for mpr in "$APPS_DIR"/*/*.mpr; do
  echo "=== $(basename $(dirname $mpr)) ==="
  echo "show odata clients;" > /tmp/show-int.mdl
  mxcli exec /tmp/show-int.mdl -p "$mpr" 2>&1 | tail -1
done
```

### 4. Interactive testing

```bash
mxcli repl -p <path-to-app>/EnquiriesManagement.mpr
```

### 5. Script-based testing

```bash
mxcli exec test-sequence.mdl -p <mpr>
```

Write operations (CREATE, DROP, GRANT/REVOKE) modify the `.mpr` file **in place**.

> **IMPORTANT:** Always run destructive tests against a **copy** of the project folder,
> never the original. The `.mpr` file references other files in the project directory,
> and objects that are DROPped cannot be recovered — there is no undo, no git history,
> and no Studio Pro autosave for `.mpr` files.
>
> ```bash
> # Before each destructive test session
> cp -r MyProject MyProject-test
> mxcli repl -p MyProject-test/MyProject.mpr
> ```

---

## 1. SHOW ODATA CLIENTS

### 1.1 List all OData clients

```
show odata clients;
```

**Expected:** All consumed OData services listed. Count matches Studio Pro.

### 1.2 Filter by module

```
show odata clients in MyModule;
```

**Expected:** Only OData clients from `MyModule`.

### 1.3 Empty module

```
show odata clients in ModuleWithNoClients;
```

**Expected:** Empty result, no error.

### 1.4 Non-existent module

```
show odata clients in NonExistentModule;
```

**Expected:** Error message.

---

## 2. DESCRIBE ODATA CLIENT

### 2.1 Describe existing client

```
describe odata client MyModule.MyODataClient;
```

**Expected:** Valid `create or modify odata client` MDL output. Includes Version, ODataVersion, MetadataUrl, Timeout, ProxyType, ServiceUrl, Authentication, Headers.

### 2.2 Non-existent client

```
describe odata client MyModule.DoesNotExist;
```

**Expected:** Clear error message.

### 2.3 All properties present

Pick a client with all properties configured. Verify DESCRIBE output includes:

| Property | Format |
|----------|--------|
| Version | String literal |
| ODataVersion | `3` or `4` |
| MetadataUrl | URL string |
| Timeout | Integer (seconds) |
| ProxyType | `default` / `none` / `custom` |
| ServiceUrl | `@ConstantName` reference |
| Authentication | `none` / `basic` / `custom` |
| Headers | Block with key-value pairs |

---

## 3. CREATE ODATA CLIENT

### 3.1 Full property set

```
create odata client MyModule.NewClient
  version '1.0'
  odata_version 4
  metadata_url 'https://api.example.com/odata/v4/$metadata'
  timeout 300
  proxy_type default
  service_url @MyModule.ServiceUrlConstant
  authentication none
  headers
    'X-Custom-Header' = 'value1',
    'Accept' = 'application/json'
  end headers;
```

**Expected:** Created. Listed in `show odata clients`. DESCRIBE matches input.

### 3.2 Minimal client

```
create odata client MyModule.MinimalClient
  metadata_url 'https://api.example.com/odata/$metadata';
```

**Expected:** Created with defaults for omitted properties.

### 3.3 With basic authentication

```
create odata client MyModule.AuthClient
  metadata_url 'https://api.example.com/odata/$metadata'
  authentication basic;
```

**Expected:** Authentication set to basic. Roundtrips via DESCRIBE.

### 3.4 Duplicate without OR MODIFY

```
create odata client MyModule.NewClient
  metadata_url 'https://api.example.com/$metadata';
create odata client MyModule.NewClient
  metadata_url 'https://api.example.com/$metadata';
```

**Expected:** Second CREATE fails with "already exists" error.

### 3.5 Module auto-creation

```
create odata client NewModule.TestClient
  metadata_url 'https://api.example.com/$metadata';
```

**Expected:** `NewModule` created automatically if it doesn't exist.

### 3.6 Write guard

Attempt CREATE without opening a project for writing.

**Expected:** Error about not being connected.

---

## 4. CREATE OR MODIFY ODATA CLIENT

### 4.1 Upsert — new client

```
create or modify odata client MyModule.UpsertClient
  metadata_url 'https://api.example.com/$metadata'
  timeout 60;
```

**Expected:** Created. No error.

### 4.2 Upsert — existing client

```
create or modify odata client MyModule.UpsertClient
  metadata_url 'https://api.example.com/v2/$metadata'
  timeout 120;
```

**Expected:** Updated. ID reused. DESCRIBE shows new values.

---

## 5. ALTER ODATA CLIENT

### 5.1 Set single property

```
alter odata client MyModule.ExistingClient set timeout = 600;
```

**Expected:** Only timeout changed. Other properties unchanged.

### 5.2 Set metadata URL

```
alter odata client MyModule.ExistingClient set metadata_url = 'https://new.example.com/$metadata';
```

**Expected:** URL updated. DESCRIBE confirms.

### 5.3 Alter non-existent client

```
alter odata client MyModule.DoesNotExist set timeout = 60;
```

**Expected:** Clear error.

---

## 6. DROP ODATA CLIENT

### 6.1 Drop existing client

```
create odata client MyModule.ToDrop
  metadata_url 'https://example.com/$metadata';
drop odata client MyModule.ToDrop;
```

**Expected:** Removed. Not in `show odata clients`.

### 6.2 Cascade — deletes external entities

1. CREATE odata client
2. CREATE external entity from that client
3. DROP odata client

**Expected:** External entities sourced from this client also removed.

### 6.3 Drop non-existent client

```
drop odata client MyModule.DoesNotExist;
```

**Expected:** Clear error.

### 6.4 Write guard

**Expected:** Error if no project open for writing.

---

## 7. SHOW ODATA SERVICES

### 7.1 List all published OData services

```
show odata services;
```

**Expected:** All published OData services listed. Count matches Studio Pro.

### 7.2 Filter by module

```
show odata services in MyModule;
```

**Expected:** Only OData services from `MyModule`.

### 7.3 Empty module

```
show odata services in ModuleWithNoServices;
```

**Expected:** Empty result, no error.

### 7.4 Non-existent module

```
show odata services in NonExistentModule;
```

**Expected:** Error message.

---

## 8. DESCRIBE ODATA SERVICE

### 8.1 Describe existing service

```
describe odata service MyModule.MyODataService;
```

**Expected:** Valid `create or modify odata service` MDL output. Includes Path, Version, ODataVersion, Namespace, Authentication, published entities with expose blocks.

### 8.2 Non-existent service

```
describe odata service MyModule.DoesNotExist;
```

**Expected:** Clear error message.

### 8.3 Published entity details

Verify DESCRIBE output for a service with published entities includes:

| Property | Format |
|----------|--------|
| PUBLISH ENTITY | Qualified entity name |
| EXPOSE block | Attribute list with types |
| ReadMode | `default` / `fromDatabase` / `fromMicroflow` |
| InsertMode | `default` / `fromMicroflow` |
| UpdateMode | `default` / `fromMicroflow` |
| DeleteMode | `default` / `fromMicroflow` |
| UsePaging | `true` / `false` |
| PageSize | Integer |

---

## 9. CREATE ODATA SERVICE

### 9.1 Full service with published entity

```
create odata service MyModule.NewService
  path '/odata/v1/myservice'
  version '1.0.0'
  odata_version 4
  namespace 'com.example.myservice'
  authentication
    none
  end authentication
  publish entity MyModule.Customer
    expose
      Name,
      Email,
      PhoneNumber
    end expose
    read_mode default
    insert_mode default
    update_mode default
    delete_mode default
    use_paging true
    page_size 20;
```

**Expected:** Created. Listed in `show odata services`. DESCRIBE matches input.

### 9.2 Multiple published entities

```
create odata service MyModule.MultiEntityService
  path '/odata/v1/multi'
  version '1.0.0'
  odata_version 4
  namespace 'com.example.multi'
  authentication none end authentication
  publish entity MyModule.Customer
    expose Name, Email end expose
    read_mode default
    use_paging true
    page_size 50
  publish entity MyModule.Order
    expose OrderNumber, TotalAmount end expose
    read_mode default
    use_paging true
    page_size 100;
```

**Expected:** Both entities published. DESCRIBE lists both.

### 9.3 Duplicate without OR MODIFY

```
create odata service MyModule.NewService
  path '/odata/v1/svc'
  version '1.0.0'
  odata_version 4
  namespace 'com.example';
create odata service MyModule.NewService
  path '/odata/v1/svc'
  version '1.0.0'
  odata_version 4
  namespace 'com.example';
```

**Expected:** Second CREATE fails with "already exists" error.

### 9.4 Module auto-creation

```
create odata service NewModule.TestService
  path '/odata/v1/test'
  version '1.0.0'
  odata_version 4
  namespace 'com.example.test';
```

**Expected:** `NewModule` created automatically if it doesn't exist.

### 9.5 Write guard

**Expected:** Error if no project open for writing.

---

## 10. CREATE OR MODIFY ODATA SERVICE

### 10.1 Upsert — new service

```
create or modify odata service MyModule.UpsertService
  path '/odata/v1/upsert'
  version '1.0.0'
  odata_version 4
  namespace 'com.example.upsert';
```

**Expected:** Created. No error.

### 10.2 Upsert — existing service

```
create or modify odata service MyModule.UpsertService
  path '/odata/v2/upsert'
  version '2.0.0'
  odata_version 4
  namespace 'com.example.upsert.v2';
```

**Expected:** Updated. ID reused. DESCRIBE shows new values.

---

## 11. ALTER ODATA SERVICE

### 11.1 Set single property

```
alter odata service MyModule.ExistingService set version = '2.0.0';
```

**Expected:** Only version changed. Other properties unchanged.

### 11.2 Set path

```
alter odata service MyModule.ExistingService set path = '/odata/v2/updated';
```

**Expected:** Path updated. DESCRIBE confirms.

### 11.3 Alter non-existent service

```
alter odata service MyModule.DoesNotExist set version = '1.0.0';
```

**Expected:** Clear error.

---

## 12. DROP ODATA SERVICE

### 12.1 Drop existing service

```
create odata service MyModule.ToDrop
  path '/odata/v1/drop'
  version '1.0.0'
  odata_version 4
  namespace 'com.example.drop';
drop odata service MyModule.ToDrop;
```

**Expected:** Removed. Not in `show odata services`.

### 12.2 Drop non-existent service

```
drop odata service MyModule.DoesNotExist;
```

**Expected:** Clear error.

### 12.3 Write guard

**Expected:** Error if no project open for writing.

---

## 13. GRANT / REVOKE ACCESS ON ODATA SERVICE

### 13.1 Grant to single role

```
grant access on odata service MyModule.MyService to MyModule.User;
```

**Expected:** Role granted. Verifiable via DESCRIBE or show access.

### 13.2 Grant to multiple roles

```
grant access on odata service MyModule.MyService to MyModule.User, MyModule.Admin;
```

**Expected:** Both roles added.

### 13.3 Idempotent grant

Grant same role twice.

**Expected:** No duplicate entries on second grant.

### 13.4 Revoke from role

```
revoke access on odata service MyModule.MyService from MyModule.User;
```

**Expected:** Role removed.

### 13.5 Grant on non-existent service

**Expected:** Clear error.

### 13.6 Grant with non-existent role

**Expected:** Clear error.

---

## 14. SHOW EXTERNAL ENTITIES

### 14.1 List all external entities

```
show external entities;
```

**Expected:** All external entities listed. Count matches Studio Pro.

### 14.2 Filter by module

```
show external entities in MyModule;
```

**Expected:** Only external entities from `MyModule`.

### 14.3 Empty module

```
show external entities in ModuleWithNoExternals;
```

**Expected:** Empty result, no error.

### 14.4 Non-existent module

```
show external entities in NonExistentModule;
```

**Expected:** Error message.

---

## 15. SHOW EXTERNAL ACTIONS

### 15.1 List all external actions

```
show external actions;
```

**Expected:** All external actions listed. Count matches Studio Pro.

---

## 16. DESCRIBE EXTERNAL ENTITY

### 16.1 Describe existing external entity

```
describe external entity MyModule.MyExternalEntity;
```

**Expected:** Valid MDL output. Includes source OData client, EntitySet, RemoteName, attributes, Countable, Creatable, Deletable, Updatable flags.

### 16.2 Non-existent external entity

```
describe external entity MyModule.DoesNotExist;
```

**Expected:** Clear error message.

### 16.3 All properties present

Verify DESCRIBE output includes:

| Property | Format |
|----------|--------|
| EntitySet | String |
| RemoteName | String |
| Countable | `true` / `false` |
| Creatable | `true` / `false` |
| Deletable | `true` / `false` |
| Updatable | `true` / `false` |
| Attributes | Name + type list |

---

## 17. CREATE EXTERNAL ENTITY FROM ODATA CLIENT

### 17.1 Single entity with full properties

```
create external entity MyModule.ExternalCustomer
  from odata client MyModule.MyODataClient
  entity_set 'Customers'
  remote_name 'Customer'
  countable true
  creatable true
  deletable false
  updatable true
  attributes
    Name : String,
    Email : String,
    Age : Integer,
    Active : Boolean
  end attributes;
```

**Expected:** Created. Listed in `show external entities`. DESCRIBE matches input.

### 17.2 Minimal entity

```
create external entity MyModule.ExternalOrder
  from odata client MyModule.MyODataClient
  entity_set 'Orders'
  remote_name 'Order'
  attributes
    OrderId : Integer,
    Total : Decimal
  end attributes;
```

**Expected:** Created with defaults for omitted boolean flags.

### 17.3 Non-existent source client

```
create external entity MyModule.BadSource
  from odata client MyModule.NonExistentClient
  entity_set 'Items'
  remote_name 'Item'
  attributes
    Id : Integer
  end attributes;
```

**Expected:** Error — source OData client not found.

### 17.4 Write guard

**Expected:** Error if no project open for writing.

---

## 18. CREATE OR MODIFY EXTERNAL ENTITY

### 18.1 Upsert — new entity

```
create or modify external entity MyModule.UpsertExternal
  from odata client MyModule.MyODataClient
  entity_set 'Products'
  remote_name 'Product'
  attributes
    ProductId : Integer,
    Name : String
  end attributes;
```

**Expected:** Created. No error.

### 18.2 Upsert — existing entity

```
create or modify external entity MyModule.UpsertExternal
  from odata client MyModule.MyODataClient
  entity_set 'Products'
  remote_name 'Product'
  countable true
  attributes
    ProductId : Integer,
    Name : String,
    Price : Decimal
  end attributes;
```

**Expected:** Updated. ID reused. DESCRIBE shows new attributes.

---

## 19. CREATE EXTERNAL ENTITIES FROM CLIENT (bulk import)

### 19.1 Bulk import all entities

```
create external entities from odata client MyModule.MyODataClient;
```

**Expected:** All entities from the OData service metadata imported. Each listed in `show external entities`.

### 19.2 Selective import into target module

```
create or modify external entities
  from odata client MyModule.MyODataClient
  entities (Customer, Order, Product)
  into TargetModule;
```

**Expected:** Only Customer, Order, Product imported. Placed in `TargetModule`.

### 19.3 Non-existent entity name in list

```
create or modify external entities
  from odata client MyModule.MyODataClient
  entities (Customer, NonExistentEntity)
  into TargetModule;
```

**Expected:** Error for NonExistentEntity. Customer may or may not be created depending on fail-fast behavior.

### 19.4 Non-existent source client

```
create external entities from odata client MyModule.NonExistent;
```

**Expected:** Clear error.

---

## 20. SHOW PUBLISHED REST SERVICES

> **Feature gate:** Mendix 10.0+. Test against Evora (10.24.15) and Lato Enquiry (11.4.0).

### 20.1 List all published REST services

```
show published rest services;
```

**Expected:** All published REST services listed. Count matches Studio Pro.

### 20.2 Filter by module

```
show published rest services in MyModule;
```

**Expected:** Only REST services from `MyModule`.

### 20.3 Empty module

```
show published rest services in ModuleWithNoREST;
```

**Expected:** Empty result, no error.

### 20.4 Non-existent module

```
show published rest services in NonExistentModule;
```

**Expected:** Error message.

---

## 21. DESCRIBE PUBLISHED REST SERVICE

### 21.1 Describe existing service

```
describe published rest service MyModule.MyRestService;
```

**Expected:** Valid `create or replace published rest service` MDL output. Includes Path, Version, ServiceName, resource blocks with HTTP methods and microflow references.

### 21.2 Non-existent service

```
describe published rest service MyModule.DoesNotExist;
```

**Expected:** Clear error message.

### 21.3 Resource detail

Verify DESCRIBE includes for each resource:

| Property | Format |
|----------|--------|
| Resource path | String |
| HTTP method | `GET` / `POST` / `PUT` / `PATCH` / `DELETE` |
| Microflow | Qualified `Module.Microflow` reference |
| Deprecated | `deprecated` keyword if applicable |

---

## 22. CREATE PUBLISHED REST SERVICE

### 22.1 Full service with resources

```
create published rest service MyModule.CustomerAPI
  path '/api/v1/customers'
  version '1.0.0'
  service_name 'CustomerAPI'
  resource '/customers'
    get microflow MyModule.GetCustomers
    post microflow MyModule.CreateCustomer
  end resource
  resource '/customers/{id}'
    get microflow MyModule.GetCustomerById
    put microflow MyModule.UpdateCustomer
    delete microflow MyModule.DeleteCustomer
  end resource;
```

**Expected:** Created. Listed in `show published rest services`. DESCRIBE matches input.

### 22.2 Single resource, single method

```
create published rest service MyModule.SimpleAPI
  path '/api/v1/health'
  version '1.0.0'
  service_name 'HealthCheck'
  resource '/health'
    get microflow MyModule.HealthCheck
  end resource;
```

**Expected:** Created with minimal configuration.

### 22.3 Deprecated resource method

```
create published rest service MyModule.LegacyAPI
  path '/api/v1/legacy'
  version '1.0.0'
  service_name 'LegacyAPI'
  resource '/old-endpoint'
    deprecated get microflow MyModule.OldEndpoint
  end resource;
```

**Expected:** Deprecated flag preserved. DESCRIBE shows `deprecated` keyword.

### 22.4 Duplicate without OR REPLACE

```
create published rest service MyModule.CustomerAPI
  path '/api/v1/dup'
  version '1.0.0'
  service_name 'Dup';
create published rest service MyModule.CustomerAPI
  path '/api/v1/dup'
  version '1.0.0'
  service_name 'Dup';
```

**Expected:** Second CREATE fails with "already exists" error.

### 22.5 Module auto-creation

```
create published rest service NewModule.TestAPI
  path '/api/v1/test'
  version '1.0.0'
  service_name 'TestAPI';
```

**Expected:** `NewModule` created automatically if it doesn't exist.

### 22.6 Write guard

**Expected:** Error if no project open for writing.

---

## 23. CREATE OR REPLACE PUBLISHED REST SERVICE

### 23.1 Upsert — new service

```
create or replace published rest service MyModule.UpsertAPI
  path '/api/v1/upsert'
  version '1.0.0'
  service_name 'UpsertAPI'
  resource '/items'
    get microflow MyModule.GetItems
  end resource;
```

**Expected:** Created. No error.

### 23.2 Upsert — existing service

```
create or replace published rest service MyModule.UpsertAPI
  path '/api/v2/upsert'
  version '2.0.0'
  service_name 'UpsertAPI'
  resource '/items'
    get microflow MyModule.GetItemsV2
    post microflow MyModule.CreateItem
  end resource;
```

**Expected:** Replaced. ID reused. DESCRIBE shows new values.

---

## 24. ALTER PUBLISHED REST SERVICE

### 24.1 Set property

```
alter published rest service MyModule.MyAPI set version = '2.0.0';
```

**Expected:** Only version changed. Resources unchanged.

### 24.2 Add resource

```
alter published rest service MyModule.MyAPI
  add resource '/new-resource'
    get microflow MyModule.GetNewResource
  end resource;
```

**Expected:** New resource added. Existing resources unchanged.

### 24.3 Drop resource

```
alter published rest service MyModule.MyAPI
  drop resource '/old-resource';
```

**Expected:** Resource removed. Other resources unchanged.

### 24.4 Alter non-existent service

```
alter published rest service MyModule.DoesNotExist set version = '1.0.0';
```

**Expected:** Clear error.

---

## 25. DROP PUBLISHED REST SERVICE

### 25.1 Drop existing service

```
drop published rest service MyModule.CustomerAPI;
```

**Expected:** Removed. Not in `show published rest services`.

### 25.2 Drop non-existent service

```
drop published rest service MyModule.DoesNotExist;
```

**Expected:** Clear error.

### 25.3 Write guard

**Expected:** Error if no project open for writing.

---

## 26. GRANT / REVOKE ACCESS ON PUBLISHED REST SERVICE

### 26.1 Grant to single role

```
grant access on published rest service MyModule.MyAPI to MyModule.User;
```

**Expected:** Role granted.

### 26.2 Grant to multiple roles

```
grant access on published rest service MyModule.MyAPI to MyModule.User, MyModule.Admin;
```

**Expected:** Both roles added.

### 26.3 Revoke from role

```
revoke access on published rest service MyModule.MyAPI from MyModule.User;
```

**Expected:** Role removed.

### 26.4 Grant on non-existent service

**Expected:** Clear error.

### 26.5 Grant with non-existent role

**Expected:** Clear error.

---

## 27. SHOW REST CLIENTS

> **Feature gate:** Mendix 10.1+. Test against Lato Enquiry (11.4.0) and Evora (10.24.15).

### 27.1 List all REST clients

```
show rest clients;
```

**Expected:** All consumed REST services listed. Count matches Studio Pro.

### 27.2 Filter by module

```
show rest clients in MyModule;
```

**Expected:** Only REST clients from `MyModule`.

### 27.3 Empty module

```
show rest clients in ModuleWithNoREST;
```

**Expected:** Empty result, no error.

### 27.4 Non-existent module

```
show rest clients in NonExistentModule;
```

**Expected:** Error message.

---

## 28. DESCRIBE REST CLIENT

### 28.1 Describe existing client

```
describe rest client MyModule.MyRestClient;
```

**Expected:** Valid `create or modify rest client` MDL output. Includes BaseUrl, Authentication, operation blocks.

### 28.2 Non-existent client

```
describe rest client MyModule.DoesNotExist;
```

**Expected:** Clear error message.

### 28.3 Operation detail

Verify DESCRIBE output for each operation includes:

| Property | Format |
|----------|--------|
| Method | `GET` / `POST` / `PUT` / `PATCH` / `DELETE` |
| Path | String with `{param}` placeholders |
| Parameters | Name + type list |
| Query | Key-value pairs |
| Headers | Key-value pairs |
| Body | Type reference or inline |
| Timeout | Integer (seconds) |
| Response | Type reference |

---

## 29. CREATE REST CLIENT (manual)

### 29.1 Full client with operations

```
create rest client MyModule.ExternalAPI
  base_url 'https://api.example.com/v1'
  authentication none
  operation GetUsers
    method GET
    path '/users'
    timeout 30
    response MyModule.UserListResponse
  end operation
  operation CreateUser
    method POST
    path '/users'
    headers
      'Content-Type' = 'application/json'
    end headers
    body MyModule.CreateUserRequest
    timeout 60
    response MyModule.UserResponse
  end operation
  operation GetUserById
    method GET
    path '/users/{userId}'
    parameters
      userId : String
    end parameters
    timeout 30
    response MyModule.UserResponse
  end operation;
```

**Expected:** Created. Listed in `show rest clients`. DESCRIBE matches input.

### 29.2 With basic authentication

```
create rest client MyModule.AuthAPI
  base_url 'https://secure.example.com/api'
  authentication basic
  operation Ping
    method GET
    path '/ping'
    timeout 10
  end operation;
```

**Expected:** Authentication set to basic. Roundtrips via DESCRIBE.

### 29.3 With query parameters

```
create rest client MyModule.SearchAPI
  base_url 'https://api.example.com'
  authentication none
  operation Search
    method GET
    path '/search'
    query
      'q' = '$SearchTerm',
      'limit' = '10'
    end query
    timeout 30
  end operation;
```

**Expected:** Query parameters preserved in DESCRIBE.

### 29.4 Duplicate without OR MODIFY

```
create rest client MyModule.ExternalAPI
  base_url 'https://api.example.com';
create rest client MyModule.ExternalAPI
  base_url 'https://api.example.com';
```

**Expected:** Second CREATE fails with "already exists" error.

### 29.5 Write guard

**Expected:** Error if no project open for writing.

---

## 30. CREATE REST CLIENT FROM OPENAPI

### 30.1 From local file

```
create rest client MyModule.PetStore
  from openapi '/path/to/petstore.yaml';
```

**Expected:** Client created with operations derived from OpenAPI spec. All paths and methods mapped.

### 30.2 From URL

```
create rest client MyModule.PetStoreURL
  from openapi 'https://petstore3.swagger.io/api/v3/openapi.json';
```

**Expected:** Spec fetched and client created. Operations match spec paths.

### 30.3 Invalid spec

```
create rest client MyModule.BadSpec
  from openapi '/path/to/invalid.yaml';
```

**Expected:** Clear error about spec parsing failure.

### 30.4 Non-existent file

```
create rest client MyModule.NoFile
  from openapi '/nonexistent/spec.yaml';
```

**Expected:** Clear error about file not found.

---

## 31. CREATE OR MODIFY REST CLIENT

### 31.1 Upsert — new client

```
create or modify rest client MyModule.UpsertREST
  base_url 'https://api.example.com'
  authentication none;
```

**Expected:** Created. No error.

### 31.2 Upsert — existing client

```
create or modify rest client MyModule.UpsertREST
  base_url 'https://api.example.com/v2'
  authentication basic;
```

**Expected:** Updated. ID reused. DESCRIBE shows new values.

---

## 32. DROP REST CLIENT

### 32.1 Drop existing client

```
drop rest client MyModule.ExternalAPI;
```

**Expected:** Removed. Not in `show rest clients`.

### 32.2 Drop non-existent client

```
drop rest client MyModule.DoesNotExist;
```

**Expected:** Clear error.

### 32.3 Write guard

**Expected:** Error if no project open for writing.

---

## 33. DESCRIBE CONTRACT OPERATION FROM OPENAPI

### 33.1 Preview from local file

```
describe contract operation from openapi '/path/to/petstore.yaml';
```

**Expected:** Lists all operations from the spec without creating anything. Shows method, path, parameters, request/response types.

### 33.2 Preview from URL

```
describe contract operation from openapi 'https://petstore3.swagger.io/api/v3/openapi.json';
```

**Expected:** Same as §33.1 but fetched from URL.

### 33.3 Invalid spec

```
describe contract operation from openapi '/path/to/invalid.yaml';
```

**Expected:** Clear error about spec parsing failure.

---

## 34. SHOW JSON STRUCTURES

### 34.1 List all JSON structures

```
show json structures;
```

**Expected:** All JSON structures listed. Count matches Studio Pro.

### 34.2 Filter by module

```
show json structures in MyModule;
```

**Expected:** Only JSON structures from `MyModule`.

### 34.3 Empty module

```
show json structures in ModuleWithNoJSON;
```

**Expected:** Empty result, no error.

### 34.4 Non-existent module

```
show json structures in NonExistentModule;
```

**Expected:** Error message.

---

## 35. DESCRIBE JSON STRUCTURE

### 35.1 Describe existing structure

```
describe json structure MyModule.MyJsonStructure;
```

**Expected:** Valid `create or replace json structure` MDL output. Includes JSON snippet, folder, comment, custom name map.

### 35.2 Non-existent structure

```
describe json structure MyModule.DoesNotExist;
```

**Expected:** Clear error message.

---

## 36. CREATE JSON STRUCTURE

### 36.1 With single-quoted snippet

```
create json structure MyModule.UserSchema
  '{"name": "string", "age": 0, "active": true}';
```

**Expected:** Created. Listed in `show json structures`. DESCRIBE matches input.

### 36.2 With dollar-quoting

```
create json structure MyModule.ComplexSchema
  $$
  {
    "users": [
      {
        "id": 0,
        "name": "string",
        "email": "string",
        "address": {
          "street": "string",
          "city": "string",
          "zip": "string"
        }
      }
    ],
    "total": 0,
    "page": 0
  }
  $$;
```

**Expected:** Created. Complex nested structure preserved. DESCRIBE shows full JSON.

### 36.3 With folder and comment

```
create json structure MyModule.DocumentedSchema
  folder 'API/Schemas'
  comment 'Schema for the user creation endpoint'
  '{"username": "string", "password": "string"}';
```

**Expected:** Folder and comment preserved in DESCRIBE.

### 36.4 With custom name map

```
create json structure MyModule.MappedSchema
  '{"user-name": "string", "e-mail": "string"}'
  custom name map
    'user-name' = 'UserName',
    'e-mail' = 'Email'
  end custom name map;
```

**Expected:** Custom name map preserved. DESCRIBE shows mappings.

### 36.5 Duplicate without OR REPLACE

```
create json structure MyModule.UserSchema '{"a": 1}';
create json structure MyModule.UserSchema '{"a": 1}';
```

**Expected:** Second CREATE fails with "already exists" error.

### 36.6 Module auto-creation

```
create json structure NewModule.TestSchema '{"test": true}';
```

**Expected:** `NewModule` created automatically if it doesn't exist.

### 36.7 Write guard

**Expected:** Error if no project open for writing.

### 36.8 Invalid JSON snippet

```
create json structure MyModule.BadJSON '{invalid json}';
```

**Expected:** Clear error about JSON parsing failure.

---

## 37. CREATE OR REPLACE JSON STRUCTURE

### 37.1 Upsert — new structure

```
create or replace json structure MyModule.UpsertSchema
  '{"value": "string"}';
```

**Expected:** Created. No error.

### 37.2 Upsert — existing structure

```
create or replace json structure MyModule.UpsertSchema
  '{"value": "string", "count": 0}';
```

**Expected:** Replaced. DESCRIBE shows new schema.

---

## 38. DROP JSON STRUCTURE

### 38.1 Drop existing structure

```
drop json structure MyModule.UserSchema;
```

**Expected:** Removed. Not in `show json structures`.

### 38.2 Drop non-existent structure

```
drop json structure MyModule.DoesNotExist;
```

**Expected:** Clear error.

### 38.3 Write guard

**Expected:** Error if no project open for writing.

---

## 39. ROUNDTRIP (BSON)

### 39.1 OData Client roundtrip

1. DESCRIBE odata client → capture MDL
2. DROP odata client
3. Execute captured MDL
4. DESCRIBE again → capture
5. Diff the two outputs

**Expected:** Identical or cosmetic-only differences.

### 39.2 OData Service roundtrip

1. DESCRIBE odata service → capture MDL
2. DROP odata service
3. Execute captured MDL
4. DESCRIBE again → capture
5. Diff

**Expected:** Identical or cosmetic-only differences.

### 39.3 External Entity roundtrip

1. DESCRIBE external entity → capture MDL
2. Execute captured MDL via CREATE OR MODIFY
3. DESCRIBE again → capture
4. Diff

**Expected:** Identical output.

### 39.4 Published REST Service roundtrip

1. DESCRIBE published rest service → capture MDL
2. DROP published rest service
3. Execute captured MDL
4. DESCRIBE again → capture
5. Diff

**Expected:** Identical or cosmetic-only differences.

### 39.5 REST Client roundtrip

1. DESCRIBE rest client → capture MDL
2. DROP rest client
3. Execute captured MDL
4. DESCRIBE again → capture
5. Diff

**Expected:** Identical or cosmetic-only differences.

### 39.6 JSON Structure roundtrip

1. DESCRIBE json structure → capture MDL
2. DROP json structure
3. Execute captured MDL
4. DESCRIBE again → capture
5. Diff

**Expected:** Identical output. JSON whitespace normalization acceptable.

### 39.7 Bulk roundtrip

Run §39.1–§39.6 on all instances across 3 test projects. Record pass/fail counts.

---

## 40. MULTI-STEP WORKFLOWS

### 40.1 End-to-end REST integration

1. CREATE json structure for request/response schemas
2. CREATE rest client with operations referencing the JSON structures
3. CREATE microflow calling the REST client operation
4. DESCRIBE each artifact — verify cross-references intact

### 40.2 OData publish-consume loop

1. CREATE odata service publishing entity `MyModule.Product`
2. CREATE odata client pointing to the service metadata URL
3. CREATE external entity from the OData client
4. DESCRIBE each — verify chain intact

### 40.3 Scaffold module with all integration types

1. CREATE odata client in `IntegrationModule`
2. CREATE external entities from client
3. CREATE published rest service exposing data
4. CREATE json structure for API contract
5. SHOW each type in `IntegrationModule` — verify all present
6. DESCRIBE each — verify complete MDL output

### 40.4 Iterative service evolution

1. CREATE published rest service with 1 resource, 1 method
2. ALTER to add second resource
3. ALTER to add method to first resource
4. ALTER to set new version
5. After each step: DESCRIBE and verify cumulative changes

### 40.5 Drop and recreate with different configuration

1. CREATE rest client with 2 operations
2. DROP
3. CREATE same name with 3 different operations
4. DESCRIBE — verify new configuration, no remnant of old operations

---

## 41. FAILURE MODES

### 41.1 Not connected

Attempt each write command without opening a project:

| Command | Expected |
|---------|----------|
| `create odata client ...` | Error: not connected |
| `create odata service ...` | Error: not connected |
| `create external entity ...` | Error: not connected |
| `create published rest service ...` | Error: not connected |
| `create rest client ...` | Error: not connected |
| `create json structure ...` | Error: not connected |

### 41.2 Entity not found

```
create odata service MyModule.BadService
  path '/odata/v1/bad'
  version '1.0.0'
  odata_version 4
  namespace 'com.example'
  publish entity MyModule.NonExistentEntity
    expose Name end expose;
```

**Expected:** Error — entity not found.

### 41.3 Duplicate without OR MODIFY/REPLACE

Test for each type. See §3.4, §9.3, §22.4, §29.4, §36.5.

### 41.4 Invalid metadata URL

```
create odata client MyModule.BadUrl
  metadata_url 'not-a-url';
```

**Expected:** Error about invalid URL format or connection failure.

### 41.5 Unsupported driver version

Open a pre-10.0 `.mpr` file. Attempt:

```
show published rest services;
```

**Expected:** Clear error about feature gate — Published REST requires Mendix 10.0+.

### 41.6 Feature gate violation — REST client

Open a pre-10.1 `.mpr` file. Attempt:

```
show rest clients;
```

**Expected:** Clear error about feature gate — REST Clients require Mendix 10.1+.

### 41.7 Invalid JSON in structure

```
create json structure MyModule.Bad '{not json at all';
```

**Expected:** Clear error about JSON syntax.

### 41.8 Non-existent microflow in REST service

```
create published rest service MyModule.BadRef
  path '/api/v1/bad'
  version '1.0.0'
  service_name 'BadRef'
  resource '/items'
    get microflow MyModule.NonExistentMicroflow
  end resource;
```

**Expected:** Error — microflow not found.

### 41.9 Double DROP

```
drop rest client MyModule.X;
drop rest client MyModule.X;
```

**Expected:** First succeeds, second gives "not found" error.

### 41.10 Validation failure mid-batch

```
create json structure MyModule.Good1 '{"ok": true}';
create json structure MyModule.Bad '{invalid}';
create json structure MyModule.Good3 '{"ok": true}';
```

**Expected:** Good1 created, Bad rejected, Good3 NOT created — batch aborts on first error.

> Batch mode (`mxcli exec`) is fail-fast. REPL mode continues on error per-line.

### 41.11 Error message quality

For each error scenario, verify the message includes:
- **What** went wrong
- **Which** object (qualified name)
- **Actionable guidance** where applicable

---

## Test Project Coverage Matrix

| Category | Enquiries (11.4.0) | Evora Factory (10.24.15) | Lato Inventory (11.2.0) |
|---|---|---|---|
| SHOW OData clients | Verify count | Verify count | Verify count |
| SHOW OData services | Verify count | Verify count | Verify count |
| SHOW external entities | Verify count | Verify count | Verify count |
| SHOW published REST | Verify count | Verify count | Verify count |
| SHOW REST clients | Verify count | Verify count | Verify count |
| SHOW JSON structures | Verify count | Verify count | Verify count |
| DESCRIBE (sample 5+ each) | All 6 types | All 6 types | All 6 types |
| Roundtrip (sample 5+ each) | All 6 types | All 6 types | All 6 types |
| CREATE + DROP cycle | All 6 types | All 6 types | All 6 types |
| Feature gate (REST) | ✅ 10.0+ | ✅ 10.0+ | ✅ 10.0+ |
| Feature gate (REST client) | ✅ 10.1+ | ✅ 10.1+ | ✅ 10.1+ |
| Multi-step workflows (§40) | End-to-end | End-to-end | End-to-end |
| Security (GRANT/REVOKE) | OData + REST | OData + REST | OData + REST |

---

## Automated Test Coverage

| Area | Tests | Status |
|---|---|---|
| SHOW ODATA CLIENTS | None | **Gap** |
| DESCRIBE ODATA CLIENT | None | **Gap** |
| CREATE ODATA CLIENT | None | **Gap** |
| DROP ODATA CLIENT | None | **Gap** |
| SHOW ODATA SERVICES | None | **Gap** |
| DESCRIBE ODATA SERVICE | None | **Gap** |
| CREATE ODATA SERVICE | None | **Gap** |
| DROP ODATA SERVICE | None | **Gap** |
| GRANT/REVOKE ODATA SERVICE | None | **Gap** |
| SHOW EXTERNAL ENTITIES | None | **Gap** |
| DESCRIBE EXTERNAL ENTITY | None | **Gap** |
| CREATE EXTERNAL ENTITY | None | **Gap** |
| CREATE EXTERNAL ENTITIES (bulk) | None | **Gap** |
| SHOW PUBLISHED REST SERVICES | None | **Gap** |
| DESCRIBE PUBLISHED REST SERVICE | None | **Gap** |
| CREATE PUBLISHED REST SERVICE | None | **Gap** |
| DROP PUBLISHED REST SERVICE | None | **Gap** |
| GRANT/REVOKE REST SERVICE | None | **Gap** |
| SHOW REST CLIENTS | None | **Gap** |
| DESCRIBE REST CLIENT | None | **Gap** |
| CREATE REST CLIENT (manual) | None | **Gap** |
| CREATE REST CLIENT (OpenAPI) | None | **Gap** |
| DROP REST CLIENT | None | **Gap** |
| DESCRIBE CONTRACT OPERATION | None | **Gap** |
| SHOW JSON STRUCTURES | None | **Gap** |
| DESCRIBE JSON STRUCTURE | None | **Gap** |
| CREATE JSON STRUCTURE | None | **Gap** |
| DROP JSON STRUCTURE | None | **Gap** |
| Roundtrip (all 6 types) | None | **Gap** |
| Multi-step workflows (§40) | None | **Manual only** |
| Failure modes (§41) | None | **Manual only** |

Manual testing priority:
1. Roundtrip all instances across 3 projects (bulk DESCRIBE→DROP→CREATE→DESCRIBE)
2. Feature gate enforcement (pre-10.0, pre-10.1 projects)
3. Multi-step workflows (§40) — highest interaction bug risk
4. Failure modes (§41) — especially §41.5, §41.6, §41.10
5. GRANT/REVOKE on OData services and Published REST services

---

## Manual Test Report Template

Copy and fill in after running manual tests.

```markdown
## Manual Testing

**Date:** YYYY-MM-DD
**Build:** `make build && make test && make lint-go` — PASS

### Test Projects

| App | Studio Pro | SHOW counts (clients/services/entities/REST/JSON) | DESCRIBE sample | Roundtrip |
|-----|-----------|---------------------------------------------------|-----------------|-----------|
| Lato Enquiry Management | 11.4.0 | ✅ _/_/_/_/_ | ✅ _n_ tested | ✅ _n_ passed |
| Evora Factory Management | 10.24.15 | ✅ _/_/_/_/_ | ✅ _n_ tested | ✅ _n_ passed |
| Lato Product Inventory | 11.2.0 | ✅ _/_/_/_/_ | ✅ _n_ tested | ✅ _n_ passed |

### OData Client Coverage

| Command | Tested | Notes |
|---------|--------|-------|
| SHOW ODATA CLIENTS | ✅/❌ | |
| SHOW ODATA CLIENTS IN module | ✅/❌ | |
| DESCRIBE ODATA CLIENT | ✅/❌ | |
| CREATE ODATA CLIENT | ✅/❌ | |
| CREATE OR MODIFY ODATA CLIENT | ✅/❌ | |
| ALTER ODATA CLIENT | ✅/❌ | |
| DROP ODATA CLIENT | ✅/❌ | |

### OData Service Coverage

| Command | Tested | Notes |
|---------|--------|-------|
| SHOW ODATA SERVICES | ✅/❌ | |
| SHOW ODATA SERVICES IN module | ✅/❌ | |
| DESCRIBE ODATA SERVICE | ✅/❌ | |
| CREATE ODATA SERVICE | ✅/❌ | |
| CREATE OR MODIFY ODATA SERVICE | ✅/❌ | |
| ALTER ODATA SERVICE | ✅/❌ | |
| DROP ODATA SERVICE | ✅/❌ | |
| GRANT ACCESS ON ODATA SERVICE | ✅/❌ | |
| REVOKE ACCESS ON ODATA SERVICE | ✅/❌ | |

### External Entity Coverage

| Command | Tested | Notes |
|---------|--------|-------|
| SHOW EXTERNAL ENTITIES | ✅/❌ | |
| SHOW EXTERNAL ENTITIES IN module | ✅/❌ | |
| SHOW EXTERNAL ACTIONS | ✅/❌ | |
| DESCRIBE EXTERNAL ENTITY | ✅/❌ | |
| CREATE EXTERNAL ENTITY | ✅/❌ | |
| CREATE OR MODIFY EXTERNAL ENTITY | ✅/❌ | |
| CREATE EXTERNAL ENTITIES (bulk) | ✅/❌ | |
| CREATE OR MODIFY EXTERNAL ENTITIES (selective) | ✅/❌ | |

### Published REST Service Coverage

| Command | Tested | Notes |
|---------|--------|-------|
| SHOW PUBLISHED REST SERVICES | ✅/❌ | |
| SHOW PUBLISHED REST SERVICES IN module | ✅/❌ | |
| DESCRIBE PUBLISHED REST SERVICE | ✅/❌ | |
| CREATE PUBLISHED REST SERVICE | ✅/❌ | |
| CREATE OR REPLACE PUBLISHED REST SERVICE | ✅/❌ | |
| ALTER PUBLISHED REST SERVICE SET | ✅/❌ | |
| ALTER PUBLISHED REST SERVICE ADD RESOURCE | ✅/❌ | |
| ALTER PUBLISHED REST SERVICE DROP RESOURCE | ✅/❌ | |
| DROP PUBLISHED REST SERVICE | ✅/❌ | |
| GRANT ACCESS ON PUBLISHED REST SERVICE | ✅/❌ | |
| REVOKE ACCESS ON PUBLISHED REST SERVICE | ✅/❌ | |

### REST Client Coverage

| Command | Tested | Notes |
|---------|--------|-------|
| SHOW REST CLIENTS | ✅/❌ | |
| SHOW REST CLIENTS IN module | ✅/❌ | |
| DESCRIBE REST CLIENT | ✅/❌ | |
| CREATE REST CLIENT (manual) | ✅/❌ | |
| CREATE REST CLIENT FROM OPENAPI (file) | ✅/❌ | |
| CREATE REST CLIENT FROM OPENAPI (URL) | ✅/❌ | |
| CREATE OR MODIFY REST CLIENT | ✅/❌ | |
| DROP REST CLIENT | ✅/❌ | |
| DESCRIBE CONTRACT OPERATION FROM OPENAPI | ✅/❌ | |

### JSON Structure Coverage

| Command | Tested | Notes |
|---------|--------|-------|
| SHOW JSON STRUCTURES | ✅/❌ | |
| SHOW JSON STRUCTURES IN module | ✅/❌ | |
| DESCRIBE JSON STRUCTURE | ✅/❌ | |
| CREATE JSON STRUCTURE (single-quote) | ✅/❌ | |
| CREATE JSON STRUCTURE (dollar-quoting) | ✅/❌ | |
| CREATE JSON STRUCTURE (with folder/comment) | ✅/❌ | |
| CREATE JSON STRUCTURE (custom name map) | ✅/❌ | |
| CREATE OR REPLACE JSON STRUCTURE | ✅/❌ | |
| DROP JSON STRUCTURE | ✅/❌ | |

### Roundtrip Results

```
OData Clients:   _n_ tested, _n_ passed, _n_ failed
OData Services:  _n_ tested, _n_ passed, _n_ failed
External Entities: _n_ tested, _n_ passed, _n_ failed
Published REST:  _n_ tested, _n_ passed, _n_ failed
REST Clients:    _n_ tested, _n_ passed, _n_ failed
JSON Structures: _n_ tested, _n_ passed, _n_ failed
```

### Multi-Step Workflows (§40)

| Scenario | Result | Notes |
|----------|--------|-------|
| 40.1 End-to-end REST integration | ✅/❌ | |
| 40.2 OData publish-consume loop | ✅/❌ | |
| 40.3 Scaffold module | ✅/❌ | |
| 40.4 Iterative service evolution | ✅/❌ | |
| 40.5 Drop/recreate different config | ✅/❌ | |

### Failure Modes (§41)

| Scenario | Result | Notes |
|----------|--------|-------|
| 41.1 Not connected (all 6 types) | ✅/❌ | |
| 41.2 Entity not found | ✅/❌ | |
| 41.4 Invalid metadata URL | ✅/❌ | |
| 41.5 Feature gate (pre-10.0) | ✅/❌ | |
| 41.6 Feature gate (pre-10.1) | ✅/❌ | |
| 41.7 Invalid JSON | ✅/❌ | |
| 41.8 Non-existent microflow ref | ✅/❌ | |
| 41.10 Validation mid-batch | ✅/❌ | |

### Issues Found

1. (none / describe issues here)
```
